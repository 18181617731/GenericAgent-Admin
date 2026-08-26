//go:build darwin

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#include "desktop_webview_darwin.h"

static NSString *const kGADesktopHandlerName = @"gaDesktop";

static void ga_desktop_forget(int32_t id);

@interface GAWebView : WKWebView
@end

@implementation GAWebView
@end

@interface GADesktopHost : NSObject <NSWindowDelegate, WKScriptMessageHandler, WKUIDelegate>
@property(nonatomic, assign) int32_t windowID;
@property(nonatomic, strong) NSWindow *window;
@property(nonatomic, strong) WKWebView *webview;
@end

@implementation GADesktopHost

- (void)userContentController:(WKUserContentController *)userContentController
	didReceiveScriptMessage:(WKScriptMessage *)message {
	if (![message.body isKindOfClass:[NSString class]]) {
		return;
	}
	NSString *text = (NSString *)message.body;
	const char *json = [text UTF8String];
	if (json != NULL) {
		goDesktopMessage(self.windowID, (char *)json);
	}
}

- (WKWebView *)webView:(WKWebView *)webView
	createWebViewWithConfiguration:(WKWebViewConfiguration *)configuration
	forNavigationAction:(WKNavigationAction *)navigationAction
	windowFeatures:(WKWindowFeatures *)windowFeatures {
	NSURLRequest *request = navigationAction.request;
	NSURL *url = request.URL;
	if (url != nil) {
		[[NSWorkspace sharedWorkspace] openURL:url];
	}
	return nil;
}

- (void)windowWillClose:(NSNotification *)notification {
	[self.webview.configuration.userContentController
		removeScriptMessageHandlerForName:kGADesktopHandlerName];
	self.window.delegate = nil;
	self.webview.UIDelegate = nil;
	int32_t id = self.windowID;
	ga_desktop_forget(id);
	goDesktopClosed(id);
}

@end

static NSMutableDictionary<NSNumber *, GADesktopHost *> *gaDesktopHosts;

static NSMenuItem *ga_desktop_edit_item(NSString *title, SEL action, NSString *key) {
	NSMenuItem *item = [[NSMenuItem alloc] initWithTitle:title action:action keyEquivalent:key];
	item.target = nil;
	item.keyEquivalentModifierMask = NSEventModifierFlagCommand;
	return item;
}

static void ga_desktop_install_edit_menu(void) {
	NSMenu *mainMenu = NSApp.mainMenu;
	if (mainMenu == nil) {
		mainMenu = [[NSMenu alloc] initWithTitle:@""];
		NSApp.mainMenu = mainMenu;
	}
	for (NSMenuItem *item in mainMenu.itemArray) {
		if ([item.submenu.title isEqualToString:@"Edit"]) {
			return;
		}
	}

	NSMenu *editMenu = [[NSMenu alloc] initWithTitle:@"Edit"];
	[editMenu addItem:ga_desktop_edit_item(@"Undo", @selector(undo:), @"z")];
	[editMenu addItem:ga_desktop_edit_item(@"Redo", @selector(redo:), @"Z")];
	[editMenu addItem:[NSMenuItem separatorItem]];
	[editMenu addItem:ga_desktop_edit_item(@"Cut", @selector(cut:), @"x")];
	[editMenu addItem:ga_desktop_edit_item(@"Copy", @selector(copy:), @"c")];
	[editMenu addItem:ga_desktop_edit_item(@"Paste", @selector(paste:), @"v")];
	[editMenu addItem:ga_desktop_edit_item(@"Select All", @selector(selectAll:), @"a")];

	NSMenuItem *editRoot = [[NSMenuItem alloc] initWithTitle:@"Edit" action:nil keyEquivalent:@""];
	editRoot.submenu = editMenu;
	[mainMenu addItem:editRoot];
}

static void ga_desktop_init(void) {
	static dispatch_once_t once;
	dispatch_once(&once, ^{
		gaDesktopHosts = [[NSMutableDictionary alloc] init];
	});
}

static GADesktopHost *ga_desktop_get(int32_t id) {
	ga_desktop_init();
	return gaDesktopHosts[@(id)];
}

static void ga_desktop_forget(int32_t id) {
	ga_desktop_init();
	[gaDesktopHosts removeObjectForKey:@(id)];
	if (gaDesktopHosts.count == 0) {
		[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
	}
}

static void ga_desktop_on_main(void (^block)(void)) {
	if ([NSThread isMainThread]) {
		block();
		return;
	}
	dispatch_async(dispatch_get_main_queue(), block);
}

static void ga_desktop_apply_theme(NSWindow *window, int dark) {
	if (@available(macOS 10.14, *)) {
		window.appearance = [NSAppearance
			appearanceNamed:dark ? NSAppearanceNameDarkAqua : NSAppearanceNameAqua];
	}
}

static void ga_desktop_create_on_main(
	int32_t id,
	NSString *title,
	NSString *url,
	int width,
	int height,
	int minWidth,
	int minHeight,
	int dark,
	NSString *bindNameJSON,
	NSData *iconData) {
	ga_desktop_init();

	if (NSApp == nil) {
		goDesktopFailed(id, "NSApplication is not running");
		return;
	}
	if (gaDesktopHosts[@(id)] != nil) {
		goDesktopFailed(id, "desktop window id is already in use");
		return;
	}
	ga_desktop_install_edit_menu();

	if (width <= 0) {
		width = 1280;
	}
	if (height <= 0) {
		height = 860;
	}

	WKWebViewConfiguration *config = [[WKWebViewConfiguration alloc] init];
	WKUserContentController *controller = [[WKUserContentController alloc] init];
	if (bindNameJSON.length > 0) {
		NSString *bindJS = [NSString stringWithFormat:
			@"window[%@] = function() {"
			 @"window.webkit.messageHandlers.gaDesktop.postMessage(JSON.stringify({"
			 @"method: %@,"
			 @"params: Array.prototype.slice.call(arguments)}))}",
			bindNameJSON, bindNameJSON];
		WKUserScript *bindScript = [[WKUserScript alloc]
			initWithSource:bindJS
			injectionTime:WKUserScriptInjectionTimeAtDocumentStart
			forMainFrameOnly:YES];
		[controller addUserScript:bindScript];
	}
	GADesktopHost *host = [[GADesktopHost alloc] init];
	host.windowID = id;
	[controller addScriptMessageHandler:host name:kGADesktopHandlerName];
	config.userContentController = controller;

	NSRect rect = NSMakeRect(0, 0, width, height);
	NSUInteger style = NSWindowStyleMaskTitled | NSWindowStyleMaskClosable |
		NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable;
	NSWindow *window = [[NSWindow alloc] initWithContentRect:rect
		styleMask:style
		backing:NSBackingStoreBuffered
		defer:NO];
	window.title = title.length > 0 ? title : @"GenericAgent";
	window.releasedWhenClosed = NO;
	window.restorable = NO;
	if (minWidth > 0 && minHeight > 0) {
		[window setContentMinSize:NSMakeSize(minWidth, minHeight)];
	}
	ga_desktop_apply_theme(window, dark);
	[window center];

	GAWebView *webview = [[GAWebView alloc] initWithFrame:window.contentView.bounds configuration:config];
	webview.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
	webview.UIDelegate = host;
	webview.allowsMagnification = NO;
	webview.allowsBackForwardNavigationGestures = NO;
	window.contentView = webview;

	host.window = window;
	host.webview = webview;
	window.delegate = host;
	gaDesktopHosts[@(id)] = host;

	if (iconData.length > 0) {
		NSImage *icon = [[NSImage alloc] initWithData:iconData];
		if (icon != nil) {
			[NSApp setApplicationIconImage:icon];
		}
	}

	if (url.length > 0) {
		NSURL *nsurl = [NSURL URLWithString:url];
		if (nsurl != nil) {
			[webview loadRequest:[NSURLRequest requestWithURL:nsurl]];
		}
	}

	[NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
	[window makeKeyAndOrderFront:nil];
	[NSApp activateIgnoringOtherApps:YES];
	goDesktopReady(id);
}

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
	int icon_len) {
	NSString *titleStr = title ? [NSString stringWithUTF8String:title] : @"";
	NSString *urlStr = url ? [NSString stringWithUTF8String:url] : @"";
	NSString *bindStr = bind_name_json ? [NSString stringWithUTF8String:bind_name_json] : @"";
	NSData *iconData = nil;
	if (icon != NULL && icon_len > 0) {
		iconData = [NSData dataWithBytes:icon length:(NSUInteger)icon_len];
	}
	ga_desktop_on_main(^{
		ga_desktop_create_on_main(
			id, titleStr, urlStr, width, height, min_width, min_height, dark, bindStr, iconData);
	});
}

void ga_desktop_window_focus(int32_t id) {
	ga_desktop_on_main(^{
		GADesktopHost *host = ga_desktop_get(id);
		if (host == nil) {
			return;
		}
		[NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
		if (host.window.miniaturized) {
			[host.window deminiaturize:nil];
		}
		[host.window makeKeyAndOrderFront:nil];
		[NSApp activateIgnoringOtherApps:YES];
	});
}

void ga_desktop_window_navigate(int32_t id, const char *url) {
	NSString *urlStr = url ? [NSString stringWithUTF8String:url] : @"";
	ga_desktop_on_main(^{
		GADesktopHost *host = ga_desktop_get(id);
		if (host == nil || urlStr.length == 0) {
			return;
		}
		NSURL *nsurl = [NSURL URLWithString:urlStr];
		if (nsurl == nil) {
			return;
		}
		[host.webview loadRequest:[NSURLRequest requestWithURL:nsurl]];
	});
}

void ga_desktop_window_close(int32_t id) {
	ga_desktop_on_main(^{
		GADesktopHost *host = ga_desktop_get(id);
		if (host == nil) {
			return;
		}
		[host.window performClose:nil];
	});
}

void ga_desktop_window_set_theme(int32_t id, int dark) {
	ga_desktop_on_main(^{
		GADesktopHost *host = ga_desktop_get(id);
		if (host == nil) {
			return;
		}
		ga_desktop_apply_theme(host.window, dark);
	});
}
