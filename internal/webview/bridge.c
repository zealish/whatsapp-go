#include "bridge.h"
#include "_cgo_export.h"

#include <gtk/gtk.h>
#include <webkit/webkit.h>
#include <string.h>

#define ZW_SCRIPT_HANDLER "zealish"
#define ZW_INTERNAL_HOST "web.whatsapp.com"

struct _ZwApp {
    GtkApplication *application;
    GtkWindow *window;
    WebKitWebView *web_view;
    WebKitNetworkSession *session;
    GHashTable *notifications; /* guint64 -> WebKitNotification* */
    char *url;
    char *title;
    char *data_dir;
    char *cache_dir;
    char *user_script;
    int width;
    int height;
    int maximized;
    int hidden;
    uintptr_t user_data;
};

static gboolean zw_uri_is_internal(const char *uri)
{
    if (uri == NULL)
        return FALSE;
    if (g_str_has_prefix(uri, "about:") || g_str_has_prefix(uri, "blob:") ||
        g_str_has_prefix(uri, "data:"))
        return TRUE;

    GUri *parsed = g_uri_parse(uri, G_URI_FLAGS_NONE, NULL);
    if (parsed == NULL)
        return FALSE;

    const char *host = g_uri_get_host(parsed);
    const char *scheme = g_uri_get_scheme(parsed);
    gboolean internal = FALSE;

    if (host != NULL && scheme != NULL && g_strcmp0(scheme, "https") == 0)
        internal = g_ascii_strcasecmp(host, ZW_INTERNAL_HOST) == 0 ||
                   g_str_has_suffix(host, ".whatsapp.com") ||
                   g_str_has_suffix(host, ".whatsapp.net");

    g_uri_unref(parsed);
    return internal;
}

static void zw_on_script_message(WebKitUserContentManager *manager,
                                 JSCValue *value,
                                 gpointer user_data)
{
    (void)manager;
    ZwApp *app = (ZwApp *)user_data;

    char *payload = jsc_value_to_string(value);
    if (payload != NULL) {
        zwGoMessage(app->user_data, payload);
        g_free(payload);
    }
}

static gboolean zw_on_decide_policy(WebKitWebView *web_view,
                                    WebKitPolicyDecision *decision,
                                    WebKitPolicyDecisionType type,
                                    gpointer user_data)
{
    (void)web_view;
    ZwApp *app = (ZwApp *)user_data;

    if (type != WEBKIT_POLICY_DECISION_TYPE_NAVIGATION_ACTION &&
        type != WEBKIT_POLICY_DECISION_TYPE_NEW_WINDOW_ACTION)
        return FALSE;

    WebKitNavigationPolicyDecision *nav = WEBKIT_NAVIGATION_POLICY_DECISION(decision);
    WebKitNavigationAction *action = webkit_navigation_policy_decision_get_navigation_action(nav);
    WebKitURIRequest *request = webkit_navigation_action_get_request(action);
    const char *uri = webkit_uri_request_get_uri(request);

    if (type == WEBKIT_POLICY_DECISION_TYPE_NEW_WINDOW_ACTION || !zw_uri_is_internal(uri)) {
        if (uri != NULL)
            zwGoExternalURI(app->user_data, (char *)uri);
        webkit_policy_decision_ignore(decision);
        return TRUE;
    }

    return FALSE;
}

static void zw_on_notification_closed(WebKitNotification *notification, gpointer user_data)
{
    ZwApp *app = (ZwApp *)user_data;
    guint64 id = webkit_notification_get_id(notification);

    g_hash_table_remove(app->notifications, &id);
    zwGoNotificationClosed(app->user_data, id);
}

static gboolean zw_on_show_notification(WebKitWebView *web_view,
                                        WebKitNotification *notification,
                                        gpointer user_data)
{
    (void)web_view;
    ZwApp *app = (ZwApp *)user_data;

    const char *title = webkit_notification_get_title(notification);
    const char *body = webkit_notification_get_body(notification);
    guint64 id = webkit_notification_get_id(notification);

    if (!zwGoNotification(app->user_data, id, (char *)(title ? title : ""),
                          (char *)(body ? body : "")))
        return FALSE;

    guint64 *key = g_new(guint64, 1);
    *key = id;
    g_hash_table_insert(app->notifications, key, g_object_ref(notification));
    g_signal_connect(notification, "closed", G_CALLBACK(zw_on_notification_closed), app);

    return TRUE;
}

static gboolean zw_on_permission_request(WebKitWebView *web_view,
                                         WebKitPermissionRequest *request,
                                         gpointer user_data)
{
    (void)web_view;
    (void)user_data;

    if (WEBKIT_IS_NOTIFICATION_PERMISSION_REQUEST(request) ||
        WEBKIT_IS_USER_MEDIA_PERMISSION_REQUEST(request) ||
        WEBKIT_IS_DEVICE_INFO_PERMISSION_REQUEST(request) ||
        WEBKIT_IS_CLIPBOARD_PERMISSION_REQUEST(request)) {
        webkit_permission_request_allow(request);
        return TRUE;
    }
    return FALSE;
}

static void zw_report_geometry(ZwApp *app)
{
    if (app->window == NULL)
        return;

    int maximized = gtk_window_is_maximized(app->window) ? 1 : 0;
    int width = gtk_widget_get_width(GTK_WIDGET(app->window));
    int height = gtk_widget_get_height(GTK_WIDGET(app->window));

    if (!maximized && width > 0 && height > 0) {
        app->width = width;
        app->height = height;
    }
    app->maximized = maximized;

    zwGoGeometryChanged(app->user_data, app->width, app->height, maximized);
}

static gboolean zw_on_close_request(GtkWindow *window, gpointer user_data)
{
    (void)window;
    ZwApp *app = (ZwApp *)user_data;

    zw_report_geometry(app);
    return zwGoCloseRequest(app->user_data) ? TRUE : FALSE;
}

static void zw_on_notify_geometry(GObject *object, GParamSpec *pspec, gpointer user_data)
{
    (void)object;
    (void)pspec;
    zw_report_geometry((ZwApp *)user_data);
}

static void zw_report_geometry(ZwApp *app);
static void zw_on_texture_read_finish(GObject *source_object, GAsyncResult *res, gpointer user_data)
{
    GdkClipboard *clipboard = GDK_CLIPBOARD(source_object);
    ZwApp *app = (ZwApp *)user_data;
    GError *error = NULL;

    GdkTexture *texture = gdk_clipboard_read_texture_finish(clipboard, res, &error);
    if (!texture) {
        if (error != NULL)
            g_error_free(error);
        return;
    }

    GBytes *bytes = gdk_texture_save_to_png_bytes(texture);
    g_object_unref(texture);
    if (!bytes)
        return;

    gsize size = 0;
    gconstpointer data = g_bytes_get_data(bytes, &size);
    char *b64 = g_base64_encode((const guchar *)data, size);
    g_bytes_unref(bytes);

    GString *js = g_string_new(NULL);
    g_string_append_printf(js,
        "(function() {"
        "  try {"
        "    const b64 = '%s';"
        "    if (window.__zealish && typeof window.__zealish.pasteImage === 'function') {"
        "      window.__zealish.pasteImage(b64);"
        "      return;"
        "    }"
        "    const bin = atob(b64);"
        "    const len = bin.length;"
        "    const bytes = new Uint8Array(len);"
        "    for (let i = 0; i < len; i++) bytes[i] = bin.charCodeAt(i);"
        "    const blob = new Blob([bytes], {type: 'image/png'});"
        "    const file = new File([blob], 'screenshot.png', {type: 'image/png', lastModified: Date.now()});"
        "    const dt = new DataTransfer();"
        "    dt.items.add(file);"
        "    const ev = new ClipboardEvent('paste', {"
        "      bubbles: true,"
        "      cancelable: true,"
        "      clipboardData: dt"
        "    });"
        "    const target = document.activeElement || document.querySelector('[contenteditable=\"true\"]') || document.body;"
        "    target.dispatchEvent(ev);"
        "  } catch (e) {"
        "    console.error('zealish paste error:', e);"
        "  }"
        "})();", b64);
    g_free(b64);

    if (app->web_view != NULL) {
        webkit_web_view_evaluate_javascript(app->web_view, js->str, -1, NULL, NULL, NULL, NULL, NULL);
    }
    g_string_free(js, TRUE);
}

static gboolean zw_clipboard_has_image(GdkClipboard *clipboard)
{
    if (clipboard == NULL)
        return FALSE;

    GdkContentFormats *formats = gdk_clipboard_get_formats(clipboard);
    if (formats == NULL)
        return FALSE;

    return gdk_content_formats_contain_gtype(formats, GDK_TYPE_TEXTURE) ||
           gdk_content_formats_contain_mime_type(formats, "image/png") ||
           gdk_content_formats_contain_mime_type(formats, "image/jpeg") ||
           gdk_content_formats_contain_mime_type(formats, "image/bmp") ||
           gdk_content_formats_contain_mime_type(formats, "image/tiff");
}

static void zw_trigger_paste_image(ZwApp *app)
{
    if (app->window == NULL || app->web_view == NULL)
        return;

    GdkDisplay *display = gtk_widget_get_display(GTK_WIDGET(app->window));
    if (display == NULL)
        return;

    GdkClipboard *clipboard = gdk_display_get_clipboard(display);
    if (clipboard != NULL) {
        gdk_clipboard_read_texture_async(clipboard, NULL, zw_on_texture_read_finish, app);
    }
}

static gboolean zw_on_key_pressed(GtkEventControllerKey *controller,
                                  guint keyval,
                                  guint keycode,
                                  GdkModifierType state,
                                  gpointer user_data)
{
    (void)controller;
    (void)keycode;
    ZwApp *app = (ZwApp *)user_data;
    gboolean is_ctrl = (state & GDK_CONTROL_MASK) != 0;
    if (is_ctrl && (keyval == GDK_KEY_v || keyval == GDK_KEY_V)) {
        if (app->window != NULL) {
            GdkDisplay *display = gtk_widget_get_display(GTK_WIDGET(app->window));
            if (display != NULL) {
                GdkClipboard *clipboard = gdk_display_get_clipboard(display);
                if (zw_clipboard_has_image(clipboard)) {
                    zw_trigger_paste_image(app);
                    return TRUE;
                }
            }
        }
    }
    return FALSE;
}

static void zw_on_custom_paste_action(GSimpleAction *action, GVariant *parameter, gpointer user_data)
{
    (void)action;
    (void)parameter;
    ZwApp *app = (ZwApp *)user_data;
    zw_trigger_paste_image(app);
}

static gboolean zw_on_context_menu(WebKitWebView *web_view,
                                   WebKitContextMenu *context_menu,
                                   WebKitHitTestResult *hit_test_result,
                                   gpointer user_data)
{
    (void)web_view;
    (void)hit_test_result;
    ZwApp *app = (ZwApp *)user_data;

    if (app->window == NULL)
        return FALSE;

    GdkDisplay *display = gtk_widget_get_display(GTK_WIDGET(app->window));
    if (display == NULL)
        return FALSE;

    GdkClipboard *clipboard = gdk_display_get_clipboard(display);
    if (zw_clipboard_has_image(clipboard)) {
        GList *items = webkit_context_menu_get_items(context_menu);
        for (GList *l = items; l != NULL; l = l->next) {
            WebKitContextMenuItem *item = WEBKIT_CONTEXT_MENU_ITEM(l->data);
            if (webkit_context_menu_item_get_stock_action(item) == WEBKIT_CONTEXT_MENU_ACTION_PASTE) {
                GAction *act = g_action_map_lookup_action(G_ACTION_MAP(app->window), "paste-image");
                if (act != NULL) {
                    WebKitContextMenuItem *new_item =
                        webkit_context_menu_item_new_from_gaction(act, "Paste", NULL);
                    webkit_context_menu_insert(context_menu, new_item, g_list_position(items, l));
                    webkit_context_menu_remove(context_menu, item);
                }
                break;
            }
        }
    }
    return FALSE;
}

static void zw_on_activate(GtkApplication *application, gpointer user_data)
{
    ZwApp *app = (ZwApp *)user_data;

    if (app->window != NULL) {
        gtk_window_present(app->window);
        return;
    }

    app->session = webkit_network_session_new(app->data_dir, app->cache_dir);
    WebKitCookieManager *cookies = webkit_network_session_get_cookie_manager(app->session);
    char *cookie_file = g_build_filename(app->data_dir, "cookies.sqlite", NULL);
    webkit_cookie_manager_set_persistent_storage(cookies, cookie_file,
                                                 WEBKIT_COOKIE_PERSISTENT_STORAGE_SQLITE);
    webkit_cookie_manager_set_accept_policy(cookies, WEBKIT_COOKIE_POLICY_ACCEPT_ALWAYS);
    g_free(cookie_file);

    WebKitUserContentManager *content = webkit_user_content_manager_new();
    g_signal_connect(content, "script-message-received::" ZW_SCRIPT_HANDLER,
                     G_CALLBACK(zw_on_script_message), app);
    webkit_user_content_manager_register_script_message_handler(content, ZW_SCRIPT_HANDLER, NULL);

    if (app->user_script != NULL && app->user_script[0] != '\0') {
        WebKitUserScript *script =
            webkit_user_script_new(app->user_script, WEBKIT_USER_CONTENT_INJECT_TOP_FRAME,
                                   WEBKIT_USER_SCRIPT_INJECT_AT_DOCUMENT_START, NULL, NULL);
        webkit_user_content_manager_add_script(content, script);
        webkit_user_script_unref(script);
    }

    app->web_view = WEBKIT_WEB_VIEW(g_object_new(WEBKIT_TYPE_WEB_VIEW,
                                                 "network-session", app->session,
                                                 "user-content-manager", content,
                                                 NULL));
    g_object_unref(content);

    WebKitSettings *settings = webkit_web_view_get_settings(app->web_view);
    webkit_settings_set_enable_developer_extras(settings, FALSE);
    webkit_settings_set_enable_html5_database(settings, TRUE);
    webkit_settings_set_enable_html5_local_storage(settings, TRUE);
    webkit_settings_set_enable_webrtc(settings, TRUE);
    webkit_settings_set_enable_media_stream(settings, TRUE);
    webkit_settings_set_enable_webaudio(settings, TRUE);
    webkit_settings_set_enable_media_capabilities(settings, TRUE);
    webkit_settings_set_javascript_can_access_clipboard(settings, TRUE);
    webkit_settings_set_enable_smooth_scrolling(settings, TRUE);
    webkit_settings_set_enable_back_forward_navigation_gestures(settings, FALSE);
    webkit_settings_set_media_playback_requires_user_gesture(settings, FALSE);
    webkit_settings_set_user_agent(settings, "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36");
    g_signal_connect(app->web_view, "decide-policy", G_CALLBACK(zw_on_decide_policy), app);
    g_signal_connect(app->web_view, "show-notification", G_CALLBACK(zw_on_show_notification), app);
    g_signal_connect(app->web_view, "permission-request", G_CALLBACK(zw_on_permission_request), app);
    g_signal_connect(app->web_view, "context-menu", G_CALLBACK(zw_on_context_menu), app);

    app->window = GTK_WINDOW(gtk_application_window_new(application));
    gtk_window_set_title(app->window, app->title);
    gtk_window_set_default_size(app->window, app->width, app->height);
    gtk_window_set_child(app->window, GTK_WIDGET(app->web_view));

    GSimpleAction *paste_act = g_simple_action_new("paste-image", NULL);
    g_signal_connect(paste_act, "activate", G_CALLBACK(zw_on_custom_paste_action), app);
    g_action_map_add_action(G_ACTION_MAP(app->window), G_ACTION(paste_act));

    GtkEventController *key_ctrl = gtk_event_controller_key_new();
    gtk_event_controller_set_propagation_phase(key_ctrl, GTK_PHASE_CAPTURE);
    g_signal_connect(key_ctrl, "key-pressed", G_CALLBACK(zw_on_key_pressed), app);
    gtk_widget_add_controller(GTK_WIDGET(app->window), key_ctrl);
    g_signal_connect(app->window, "close-request", G_CALLBACK(zw_on_close_request), app);
    g_signal_connect(app->window, "notify::default-width",
                     G_CALLBACK(zw_on_notify_geometry), app);
    g_signal_connect(app->window, "notify::default-height",
                     G_CALLBACK(zw_on_notify_geometry), app);
    g_signal_connect(app->window, "notify::maximized", G_CALLBACK(zw_on_notify_geometry), app);

    if (app->maximized)
        gtk_window_maximize(app->window);

    webkit_web_view_load_uri(app->web_view, app->url);

    if (!app->hidden)
        gtk_window_present(app->window);
    else
        g_application_hold(G_APPLICATION(application));
}

ZwApp *zw_app_new(const ZwConfig *cfg)
{
    ZwApp *app = g_new0(ZwApp, 1);

    app->url = g_strdup(cfg->url);
    app->title = g_strdup(cfg->title);
    app->data_dir = g_strdup(cfg->data_dir);
    app->cache_dir = g_strdup(cfg->cache_dir);
    app->user_script = g_strdup(cfg->user_script);
    app->width = cfg->width > 0 ? cfg->width : 1280;
    app->height = cfg->height > 0 ? cfg->height : 820;
    app->maximized = cfg->maximized;
    app->hidden = cfg->hidden;
    app->user_data = cfg->user_data;
    app->notifications = g_hash_table_new_full(g_int64_hash, g_int64_equal, g_free, g_object_unref);

    app->application = gtk_application_new("com.zealish.WhatsApp", G_APPLICATION_DEFAULT_FLAGS);
    if (app->application == NULL) {
        g_hash_table_unref(app->notifications);
        g_free(app);
        return NULL;
    }
    g_signal_connect(app->application, "activate", G_CALLBACK(zw_on_activate), app);

    return app;
}

int zw_app_run(ZwApp *app)
{
    return g_application_run(G_APPLICATION(app->application), 0, NULL);
}

void zw_app_quit(ZwApp *app)
{
    if (app->window != NULL)
        gtk_window_destroy(app->window);
    g_application_quit(G_APPLICATION(app->application));
}

void zw_app_show(ZwApp *app)
{
    if (app->window == NULL)
        return;
    if (app->hidden) {
        app->hidden = 0;
        g_application_release(G_APPLICATION(app->application));
    }
    gtk_window_present(app->window);
}

void zw_app_hide(ZwApp *app)
{
    if (app->window == NULL)
        return;
    if (!app->hidden) {
        app->hidden = 1;
        g_application_hold(G_APPLICATION(app->application));
    }
    gtk_widget_set_visible(GTK_WIDGET(app->window), FALSE);
}

int zw_app_visible(ZwApp *app)
{
    if (app->window == NULL)
        return 0;
    return gtk_widget_get_visible(GTK_WIDGET(app->window)) ? 1 : 0;
}

void zw_app_reload(ZwApp *app)
{
    if (app->web_view != NULL)
        webkit_web_view_reload(app->web_view);
}

void zw_app_notification_clicked(ZwApp *app, uint64_t id)
{
    guint64 key = id;
    WebKitNotification *notification = g_hash_table_lookup(app->notifications, &key);
    if (notification != NULL)
        webkit_notification_clicked(notification);
}

void zw_app_eval(ZwApp *app, const char *script)
{
    if (app->web_view != NULL)
        webkit_web_view_evaluate_javascript(app->web_view, script, -1, NULL, NULL, NULL, NULL, NULL);
}

void zw_app_geometry(ZwApp *app, int *width, int *height, int *maximized)
{
    *width = app->width;
    *height = app->height;
    *maximized = app->maximized;
}

static gboolean zw_dispatch_trampoline(gpointer data)
{
    zwGoDispatch((uintptr_t)data);
    return G_SOURCE_REMOVE;
}

void zw_dispatch(uintptr_t handle)
{
    g_idle_add(zw_dispatch_trampoline, (gpointer)handle);
}
