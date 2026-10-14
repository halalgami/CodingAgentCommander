package main

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func withDialog(t *testing.T, f func(context.Context) ([]string, error)) {
	orig := openFilesDialog
	openFilesDialog = f
	t.Cleanup(func() { openFilesDialog = orig })
}

func TestPickAttachFiles(t *testing.T) {
	a, _ := newAttachApp(t)
	withDialog(t, func(context.Context) ([]string, error) { return nil, nil }) // Cancel
	if got, err := a.PickAttachFiles(); err != nil || got == nil || len(got) != 0 {
		t.Errorf("cancel = %#v, %v; want empty non-nil", got, err)
	}
	withDialog(t, func(context.Context) ([]string, error) { return []string{"/Users/me/a.pdf", "/Users/me/b.png"}, nil })
	if got, err := a.PickAttachFiles(); err != nil || !slices.Equal(got, []string{"/Users/me/a.pdf", "/Users/me/b.png"}) {
		t.Errorf("pick = %v, %v", got, err)
	}
	withDialog(t, func(context.Context) ([]string, error) { return nil, errors.New("dialog failed") })
	if _, err := a.PickAttachFiles(); err == nil || err.Error() != "dialog failed" {
		t.Errorf("err = %v", err)
	}
}
