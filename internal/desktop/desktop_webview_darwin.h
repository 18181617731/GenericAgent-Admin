#ifndef DESKTOP_WEBVIEW_DARWIN_H
#define DESKTOP_WEBVIEW_DARWIN_H

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

// Go callbacks. cgo generates the implementations from //export functions, and
// re-declares them in _cgo_export.h next to this header. A Go *C.char always
// comes out as a plain char*, so these must not say const: the two declarations
// would then disagree and _cgo_export.h would not compile.
extern void goDesktopReady(int32_t id);
extern void goDesktopClosed(int32_t id);
extern void goDesktopFailed(int32_t id, char *err);
extern void goDesktopMessage(int32_t id, char *json);

// ga_desktop_window_create opens an NSWindow with a WKWebView on the AppKit
// main thread. title, url, and bind_name_json are copied before the call
// returns, so the Go CStrings can be freed immediately afterwards.
// bind_name_json is the JSON-quoted name of the page function (including the
// surrounding quotes), matching the Windows host's Init script.
void ga_desktop_window_create(
    int32_t id,
    const char *title,
    const char *url,
    int width,
    int height,
    int min_width,
    int min_height,
    int dark,
    const char *bind_name_json,
    const unsigned char *icon,
    int icon_len);

void ga_desktop_window_focus(int32_t id);
void ga_desktop_window_navigate(int32_t id, const char *url);
void ga_desktop_window_close(int32_t id);
void ga_desktop_window_set_theme(int32_t id, int dark);

#ifdef __cplusplus
}
#endif

#endif
