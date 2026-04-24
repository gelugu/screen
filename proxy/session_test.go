package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSessionStore_NewSession(t *testing.T) {
	store := newSessionStore()
	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	id := store.ensure(w, r)
	if id == "" {
		t.Fatal("expected non-empty session id")
	}

	var cookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("session cookie not set in response")
	}
	if cookie.Value != id {
		t.Errorf("cookie value = %q, want %q", cookie.Value, id)
	}
	if !cookie.HttpOnly {
		t.Error("cookie should be HttpOnly")
	}
}

func TestSessionStore_ReuseExisting(t *testing.T) {
	store := newSessionStore()

	r1 := httptest.NewRequest("GET", "/", nil)
	w1 := httptest.NewRecorder()
	id1 := store.ensure(w1, r1)

	r2 := httptest.NewRequest("GET", "/", nil)
	for _, c := range w1.Result().Cookies() {
		r2.AddCookie(c)
	}
	w2 := httptest.NewRecorder()
	id2 := store.ensure(w2, r2)

	if id1 != id2 {
		t.Errorf("expected same session id, got %q and %q", id1, id2)
	}
	if len(w2.Result().Cookies()) != 0 {
		t.Error("no new cookie should be set when reusing existing session")
	}
}

func TestSessionStore_VerifyBeforeSession(t *testing.T) {
	store := newSessionStore()
	r := httptest.NewRequest("GET", "/", nil)

	if store.verified(r, "site|:otp") {
		t.Error("should not be verified with no session cookie")
	}
}

func TestSessionStore_MarkAndVerify(t *testing.T) {
	store := newSessionStore()
	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	id := store.ensure(w, r)
	store.mark(id, "site|:otp")

	r2 := httptest.NewRequest("GET", "/", nil)
	for _, c := range w.Result().Cookies() {
		r2.AddCookie(c)
	}

	if !store.verified(r2, "site|:otp") {
		t.Error("expected verified after mark")
	}
}

func TestSessionStore_VerifyDifferentKey(t *testing.T) {
	store := newSessionStore()
	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	id := store.ensure(w, r)
	store.mark(id, "site|:otp")

	r2 := httptest.NewRequest("GET", "/", nil)
	for _, c := range w.Result().Cookies() {
		r2.AddCookie(c)
	}

	if store.verified(r2, "site|:captcha") {
		t.Error("should not be verified for a different key")
	}
}

func TestSessionStore_Count(t *testing.T) {
	store := newSessionStore()

	if n := store.count(); n != 0 {
		t.Fatalf("initial count = %d, want 0", n)
	}

	store.ensure(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if n := store.count(); n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}

	store.ensure(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if n := store.count(); n != 2 {
		t.Fatalf("count = %d, want 2", n)
	}
}
