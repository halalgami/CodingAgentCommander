//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation
#include <stdlib.h>
#import <Foundation/Foundation.h>

// NSUserNotification is deprecated but still delivered on current macOS and,
// unlike osascript's `display notification` (which brands every banner with
// Script Editor's icon), it posts as THIS app — so the banner carries the
// Commander icon. Returns 0 when no notification center exists (unbundled
// binary, e.g. `go test`), letting the caller fall back.
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
static int commanderPostNotification(const char *ctitle, const char *cbody) {
	@autoreleasepool {
		NSUserNotificationCenter *center =
			[NSUserNotificationCenter defaultUserNotificationCenter];
		if (center == nil) {
			return 0;
		}
		NSUserNotification *n = [[[NSUserNotification alloc] init] autorelease];
		n.title = [NSString stringWithUTF8String:ctitle];
		n.informativeText = [NSString stringWithUTF8String:cbody];
		n.soundName = @"Ping";
		dispatch_async(dispatch_get_main_queue(), ^{
			[center deliverNotification:n];
		});
		return 1;
	}
}
#pragma clang diagnostic pop
*/
import "C"

import "unsafe"

// nativeNotifier posts through the app bundle so banners show Commander's
// icon; it falls back to osascript when no notification center is available.
type nativeNotifier struct{ fallback osascriptNotifier }

func (n nativeNotifier) Notify(title, body string) error {
	ct := C.CString(title)
	cb := C.CString(body)
	defer C.free(unsafe.Pointer(ct))
	defer C.free(unsafe.Pointer(cb))
	if C.commanderPostNotification(ct, cb) == 0 {
		return n.fallback.Notify(title, body)
	}
	return nil
}
