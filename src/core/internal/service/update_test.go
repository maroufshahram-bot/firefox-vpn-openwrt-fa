package service

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/armanlife/firefox-vpn-client/core/internal/update"
)

// Only the app on this computer may download or install an update, and
// only a copy with an updater does; the status carries the update's state.
func TestUpdatesNeedTheLocalTokenAndAnUpdater(t *testing.T) {
	d := newTestDaemon(t)
	d.localToken = "update-token"
	handler := newDashboardServer(d).handler()
	send := func(path, token, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Host = "127.0.0.1:10820"
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set(localTokenHeader, token)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	for _, path := range []string{"/api/update/download", "/api/update/install"} {
		if rec := send(path, "", `{"version":"9.9.9"}`); rec.Code != http.StatusForbidden {
			t.Fatalf("%s from a browser: %d", path, rec.Code)
		}
		if rec := send(path, d.localToken, `{"version":"9.9.9"}`); rec.Code != http.StatusConflict {
			t.Fatalf("%s without an updater: %d", path, rec.Code)
		}
	}
	if view, _ := d.StatusView(); view.Update != nil {
		t.Fatalf("an update without an updater: %+v", view.Update)
	}

	app := t.TempDir()
	d.Updater = update.New(update.Options{DataDir: filepath.Join(app, "user-data"), AppDir: app, Current: "1.0.0"})
	if rec := send("/api/update/download", d.localToken, `{"version":"0.9.0"}`); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "not newer") {
		t.Fatalf("an older version: %d %s", rec.Code, rec.Body.String())
	}
	if rec := send("/api/update/install", d.localToken, `{"version":"1.2.3","pid":1}`); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "not downloaded") {
		t.Fatalf("a version not downloaded: %d %s", rec.Code, rec.Body.String())
	}
	if view, _ := d.StatusView(); view.Update == nil || view.Update.State != update.StateIdle {
		t.Fatalf("status: %+v", view.Update)
	}
}
