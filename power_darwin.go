//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation
#import <Foundation/Foundation.h>
static int commanderLowPower(void) {
	return [[NSProcessInfo processInfo] isLowPowerModeEnabled] ? 1 : 0;
}
*/
import "C"

// lowPowerMode reports macOS Low Power Mode (the dashboard goes still).
func lowPowerMode() bool { return C.commanderLowPower() == 1 }
