//go:build darwin && cgo

package api

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#include <stdlib.h>
#include <string.h>

static char *ga_choose_directory(const char *start) {
	__block char *selected = NULL;
	void (^showPanel)(void) = ^{
		@autoreleasepool {
			[NSApplication sharedApplication];
			NSOpenPanel *panel = [NSOpenPanel openPanel];
			panel.title = @"Select GenericAgent directory";
			panel.prompt = @"Choose";
			panel.canChooseFiles = NO;
			panel.canChooseDirectories = YES;
			panel.allowsMultipleSelection = NO;
			panel.canCreateDirectories = YES;

			if (start != NULL && start[0] != '\0') {
				NSString *path = [NSString stringWithUTF8String:start];
				BOOL isDirectory = NO;
				if (path != nil && [[NSFileManager defaultManager] fileExistsAtPath:path isDirectory:&isDirectory] && isDirectory) {
					panel.directoryURL = [NSURL fileURLWithPath:path isDirectory:YES];
				}
			}

			if ([panel runModal] == NSModalResponseOK) {
				const char *path = panel.URL.path.UTF8String;
				if (path != NULL) {
					selected = strdup(path);
				}
			}
		}
	};

	if ([NSThread isMainThread]) {
		showPanel();
	} else {
		dispatch_sync(dispatch_get_main_queue(), showPanel);
	}
	return selected;
}
*/
import "C"

import (
	"unsafe"
)

func chooseDirectory(start string) (string, error) {
	cStart := C.CString(start)
	defer C.free(unsafe.Pointer(cStart))

	selected := C.ga_choose_directory(cStart)
	if selected == nil {
		return "", nil
	}
	defer C.free(unsafe.Pointer(selected))
	return C.GoString(selected), nil
}
