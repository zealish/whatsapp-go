(function () {
  "use strict";

  if (window.__zealish) {
    return;
  }

  var channel = window.webkit && window.webkit.messageHandlers
    ? window.webkit.messageHandlers.zealish
    : null;

  function post(message) {
    if (channel) {
      channel.postMessage(JSON.stringify(message));
    }
  }

  var lastUnread = -1;

  function readUnread() {
    var total = 0;
    var badges = document.querySelectorAll('[aria-label][role="listitem"] span[aria-label]');
    for (var i = 0; i < badges.length; i++) {
      var value = parseInt(badges[i].textContent, 10);
      if (!isNaN(value)) {
        total += value;
      }
    }
    return total;
  }

  function reportUnread() {
    var unread = readUnread();
    if (unread !== lastUnread) {
      lastUnread = unread;
      post({ type: "unread", unread: unread });
    }
  }

  // Debounced observer: WhatsApp mutates the DOM constantly, so coalesce
  // bursts into a single read per animation frame batch.
  var pending = false;
  function schedule() {
    if (pending) {
      return;
    }
    pending = true;
    setTimeout(function () {
      pending = false;
      reportUnread();
    }, 750);
  }

  function findChatList() {
    return document.querySelector('#pane-side') || document.body;
  }

  var booted = false;
  function boot() {
    if (booted) {
      return;
    }
    var pane = document.querySelector('#pane-side');
    if (!pane) {
      return;
    }
    booted = true;

    new MutationObserver(schedule).observe(pane, {
      childList: true,
      subtree: true,
      characterData: true
    });

    post({ type: "ready" });
    reportUnread();
  }

  // Route target=_blank clicks through the host so links open in the system
  // browser instead of a detached WebKit window.
  document.addEventListener("click", function (event) {
    var anchor = event.target && event.target.closest ? event.target.closest("a[href]") : null;
    if (!anchor || anchor.target !== "_blank") {
      return;
    }
    var href = anchor.href || "";
    if (href.indexOf("http://") !== 0 && href.indexOf("https://") !== 0) {
      return;
    }
    event.preventDefault();
    post({ type: "open_external", url: href });
  }, true);

  window.__zealish = {
    focusChat: function (tag) {
      var pane = findChatList();
      var target = pane.querySelector('[aria-label="' + CSS.escape(tag) + '"]');
      if (target) {
        target.click();
      }
    },
    pasteImage: function (base64Data) {
      try {
        var binary = atob(base64Data);
        var len = binary.length;
        var bytes = new Uint8Array(len);
        for (var i = 0; i < len; i++) {
          bytes[i] = binary.charCodeAt(i);
        }
        var blob = new Blob([bytes], { type: "image/png" });
        var file = new File([blob], "screenshot.png", {
          type: "image/png",
          lastModified: Date.now()
        });
        var dataTransfer = new DataTransfer();
        dataTransfer.items.add(file);
        var pasteEvent = new ClipboardEvent("paste", {
          bubbles: true,
          cancelable: true,
          clipboardData: dataTransfer
        });
        var target =
          document.activeElement ||
          document.querySelector('[contenteditable="true"]') ||
          document.body;
        target.dispatchEvent(pasteEvent);
      } catch (err) {
        console.error("zealish pasteImage error:", err);
      }
    }
  };

  new MutationObserver(function () {
    boot();
  }).observe(document.documentElement, { childList: true, subtree: true });

  if (document.readyState !== "loading") {
    boot();
  } else {
    document.addEventListener("DOMContentLoaded", boot);
  }
})();
