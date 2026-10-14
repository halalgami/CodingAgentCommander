//go:build darwin

package main

import (
	"context"
	"os/exec"
	"time"
)

// A fixed script: nothing is interpolated into it; the destination arrives as
// argv. One read of the general pasteboard: file URLs if there are any, else
// the PNG (or TIFF) data written to <dest>.png / <dest>.tiff.
const pasteboardScript = `function run(argv) {
ObjC.import('AppKit');
var pb = argv.length > 1 ? $.NSPasteboard.pasteboardWithName($(argv[1])) : $.NSPasteboard.generalPasteboard;
var opts=$.NSDictionary.dictionaryWithObjectForKey($(true), $('NSPasteboardURLReadingFileURLsOnlyKey'));
var r=pb.readObjectsForClassesOptions($([$.NSURL]), opts);
var files=[];
if (r && !r.isNil()) { for (var i=0;i<r.count;i++) files.push(ObjC.unwrap(r.objectAtIndex(i).path)); }
var out={files:files};
if (!files.length) {
  var types=[['public.png','.png',20*1024*1024],['public.tiff','.tiff',200*1024*1024]];
  for (var k=0;k<types.length;k++) {
    var d=pb.dataForType($(types[k][0]));
    if (!d || d.isNil() || d.length<=0) continue;
    if (d.length>types[k][2]) { out.tooBig=Math.max(out.tooBig||0, d.length); continue; }
    var p=argv[0]+types[k][1];
    if (d.writeToFileAtomically($(p), true)) { out.image=p; delete out.tooBig; break; }
  }
}
return JSON.stringify(out);
}`

// clipboardRead reads the GENERAL pasteboard; readPasteboard exists so a test
// can read a private named one instead.
var clipboardRead = func(destBase string) (clipContents, error) { return readPasteboard(destBase, "") }

func readPasteboard(destBase, pasteboardName string) (clipContents, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	args := []string{"-l", "JavaScript", "-e", pasteboardScript, destBase}
	if pasteboardName != "" {
		args = append(args, pasteboardName)
	}
	out, err := exec.CommandContext(ctx, "osascript", args...).Output()
	if err != nil {
		return clipContents{}, err
	}
	return parseClipboard(out)
}
