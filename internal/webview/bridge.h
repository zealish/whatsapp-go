#ifndef ZEALISH_WA_BRIDGE_H
#define ZEALISH_WA_BRIDGE_H

#include <glib.h>
#include <stdint.h>

typedef struct _ZwApp ZwApp;

typedef struct {
    const char *title;
    const char *url;
    const char *data_dir;
    const char *cache_dir;
    const char *user_script;
    int width;
    int height;
    int maximized;
    int hidden;
    uintptr_t user_data;
} ZwConfig;

ZwApp *zw_app_new(const ZwConfig *cfg);
int zw_app_run(ZwApp *app);
void zw_app_quit(ZwApp *app);
void zw_app_show(ZwApp *app);
void zw_app_hide(ZwApp *app);
int zw_app_visible(ZwApp *app);
void zw_app_reload(ZwApp *app);
void zw_app_notification_clicked(ZwApp *app, uint64_t id);
void zw_app_eval(ZwApp *app, const char *script);
void zw_app_geometry(ZwApp *app, int *width, int *height, int *maximized);
void zw_dispatch(uintptr_t handle);

#endif /* ZEALISH_WA_BRIDGE_H */
