package server

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"net/http"
	"strings"
	"testing"

	"github.com/waifu-im/waifu-api/backend/internal/auth"
	"github.com/waifu-im/waifu-api/backend/internal/config"
)

func TestAdminAPIKeyCannotChangeRolesBansOrFiles(t *testing.T) {
	e := newTestEnv(t)
	adminTok, adminID := e.login("100000000000000101", "admin", "")
	e.setRole(adminID, auth.RoleAdmin)
	_, victimID := e.login("100000000000000102", "victim", "")

	w := e.do("POST", "/auth/api-keys", map[string]any{"description": "automation"}, withToken(adminTok))
	expectStatus(t, w, http.StatusCreated)
	key := decode[createdAPIKeyDto](t, w).Key

	rolePath := fmt.Sprintf("/users/%d/role", victimID)
	banPath := fmt.Sprintf("/users/%d/ban", victimID)
	for _, c := range []struct {
		method, path string
		body         any
	}{
		{"PUT", rolePath, map[string]any{"role": "Admin"}},
		{"PUT", banPath, map[string]any{"isBlacklisted": true}},
		{"DELETE", "/storage-diff/s3", map[string]any{"fileNames": []string{"1.png"}}},
		{"DELETE", "/storage-diff/db", map[string]any{"ids": []int64{1}}},
	} {
		w := e.do(c.method, c.path, c.body, withAPIKey(key))
		expectStatus(t, w, http.StatusForbidden)
		if !strings.Contains(w.Body.String(), "website session") {
			t.Fatalf("%s %s: unexpected body %s", c.method, c.path, w.Body.String())
		}
	}
	if role := e.scalar(`SELECT "Role" FROM "Users" WHERE "Id" = $1`, victimID); role != int64(auth.RoleUser) {
		t.Fatalf("an API key must not change roles, got %d", role)
	}

	expectStatus(t, e.do("PUT", rolePath, map[string]any{"role": "Moderator"}, withToken(adminTok)), http.StatusOK)
	expectStatus(t, e.do("GET", "/users", nil, withAPIKey(key)), http.StatusOK)
}

func TestPendingSubmissionsAreCapped(t *testing.T) {
	e := newTestEnv(t, func(c *config.Config) { c.Review.MaxPendingSubmissionsPerUser = 2 })
	tok, _ := e.login("100000000000000103", "squatter", "")
	trustedTok, trustedID := e.login("100000000000000104", "trusted", "")
	e.setRole(trustedID, auth.RoleTrustedUser)

	for i := 0; i < 2; i++ {
		expectStatus(t, e.do("POST", "/tags", map[string]any{"name": fmt.Sprintf("Squat %d", i), "description": "pending"}, withToken(tok)), http.StatusCreated)
	}
	w := e.do("POST", "/tags", map[string]any{"name": "Squat 3", "description": "pending"}, withToken(tok))
	expectStatus(t, w, http.StatusConflict)
	if !strings.Contains(w.Body.String(), "waiting for review") {
		t.Fatalf("unexpected body %s", w.Body.String())
	}
	expectStatus(t, e.do("POST", "/artists", map[string]any{"name": "Squatted artist"}, withToken(tok)), http.StatusConflict)
	w, _ = e.upload(tok, 77, nil)
	expectStatus(t, w, http.StatusConflict)
	if n := e.scalar(`SELECT COUNT(*) FROM "Images"`); n != 0 {
		t.Fatalf("a refused upload must not leave an image behind, got %d", n)
	}

	for i := 0; i < 3; i++ {
		expectStatus(t, e.do("POST", "/tags", map[string]any{"name": fmt.Sprintf("Trusted %d", i), "description": "accepted"}, withToken(trustedTok)), http.StatusCreated)
	}
}

func TestDuplicateConflictHidesImagesTheCallerCannotSee(t *testing.T) {
	e := newTestEnv(t)
	ownerTok, _ := e.login("100000000000000105", "owner", "")
	otherTok, _ := e.login("100000000000000106", "other", "")
	modTok, modID := e.login("100000000000000107", "mod", "")
	e.setRole(modID, auth.RoleModerator)

	w, img := e.upload(ownerTok, 91, nil)
	expectStatus(t, w, http.StatusCreated)

	w, _ = e.upload(otherTok, 91, nil)
	expectStatus(t, w, http.StatusConflict)
	if strings.Contains(w.Body.String(), "ID:") {
		t.Fatalf("the ID of a pending image of someone else must not leak: %s", w.Body.String())
	}
	for _, tok := range []string{ownerTok, modTok} {
		w, _ = e.upload(tok, 91, nil)
		expectStatus(t, w, http.StatusConflict)
		if !strings.Contains(w.Body.String(), fmt.Sprintf("ID: %d", img.ID)) {
			t.Fatalf("owner and moderators see the duplicate's ID: %s", w.Body.String())
		}
	}
}

func pngTextChunk(png []byte, keyword, text string) []byte {
	var chunk bytes.Buffer
	payload := append([]byte(keyword+"\x00"), text...)
	_ = binary.Write(&chunk, binary.BigEndian, uint32(len(payload)))
	chunk.WriteString("tEXt")
	chunk.Write(payload)
	_ = binary.Write(&chunk, binary.BigEndian, crc32.ChecksumIEEE(append([]byte("tEXt"), payload...)))
	out := append([]byte(nil), png[:33]...)
	out = append(out, chunk.Bytes()...)
	return append(out, png[33:]...)
}

func TestUploadRemovesMetadata(t *testing.T) {
	e := newTestEnv(t)
	tok, _ := e.login("100000000000000108", "camera", "")
	file := pngTextChunk(noisePNG(t, 92, 160, 200), "Comment", "GPS 52.52N 13.41E")

	w := e.multipart("POST", "/images/upload", uploadSpec{file: file}, withToken(tok))
	expectStatus(t, w, http.StatusCreated)
	img := decode[imageDto](t, w)
	obj, ok := e.store.get(fmt.Sprintf("%d.png", img.ID))
	if !ok {
		t.Fatal("stored object missing")
	}
	if bytes.Contains(obj.data, []byte("GPS 52.52N")) {
		t.Fatal("text metadata must be removed before storing")
	}
	if img.ByteSize != int64(len(obj.data)) {
		t.Fatalf("byteSize %d must match the stored file (%d bytes)", img.ByteSize, len(obj.data))
	}
}
