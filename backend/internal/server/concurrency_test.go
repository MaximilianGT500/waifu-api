package server

import (
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/waifu-im/waifu-api/backend/internal/auth"
)

func TestConcurrentListingDoesNotExhaustPool(t *testing.T) {
	e := newTestEnv(t)
	tok, uid := e.login("100000000000000030", "trusted", "")
	e.setRole(uid, auth.RoleTrustedUser)
	for i := int64(0); i < 12; i++ {
		w, _ := e.upload(tok, 900+i, nil)
		expectStatus(t, w, http.StatusCreated)
	}

	done := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan string, 64)
	for i := 0; i < 48; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := e.do("GET", "/images?pageSize=50&orderBy=Favorites", nil, withToken(tok))
			if w.Code != http.StatusOK {
				errs <- fmt.Sprintf("status %d: %s", w.Code, w.Body.String())
			}
		}()
	}
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("concurrent listing deadlocked")
	}
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}
}

func TestConcurrentDuplicateUploads(t *testing.T) {
	e := newTestEnv(t)
	tok, _ := e.login("100000000000000031", "dup", "")
	data := noisePNG(t, 4242, 160, 200)

	var wg sync.WaitGroup
	codes := make(chan int, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- e.multipart("POST", "/images/upload", uploadSpec{file: data}, withToken(tok)).Code
		}()
	}
	wg.Wait()
	close(codes)
	created := 0
	for c := range codes {
		switch c {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
		default:
			t.Fatalf("unexpected status %d", c)
		}
	}
	if created != 1 {
		t.Fatalf("exactly one upload must win, got %d", created)
	}
}

func TestConcurrentApproveIsSerialised(t *testing.T) {
	e := newTestEnv(t)
	userTok, _ := e.login("100000000000000032", "user", "")
	modTok, modID := e.login("100000000000000033", "mod", "")
	e.setRole(modID, auth.RoleModerator)
	_, img := e.upload(userTok, 5151, nil)
	taskID := e.scalar(`SELECT "Id" FROM "ReviewTasks" WHERE "TargetId" = $1 AND "Kind" = 0`, img.ID)

	var wg sync.WaitGroup
	codes := make(chan int, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			action := "approve"
			if i%2 == 1 {
				action = "reject"
			}
			codes <- e.do("PATCH", fmt.Sprintf("/review/tasks/%d/%s", taskID, action), nil, withToken(modTok)).Code
		}(i)
	}
	wg.Wait()
	close(codes)
	ok := 0
	for c := range codes {
		if c == http.StatusNoContent {
			ok++
		} else if c != http.StatusConflict {
			t.Fatalf("unexpected status %d", c)
		}
	}
	if ok != 1 {
		t.Fatalf("exactly one resolution must succeed, got %d", ok)
	}
}

func TestConcurrentApproveAndEditDoNotDeadlock(t *testing.T) {
	e := newTestEnv(t)
	userTok, _ := e.login("100000000000000034", "creator", "")
	modTok, modID := e.login("100000000000000035", "mod", "")
	e.setRole(modID, auth.RoleModerator)

	for round := 0; round < 15; round++ {
		w := e.do("POST", "/tags", map[string]string{"name": fmt.Sprintf("Race %d", round), "description": "race"}, withToken(userTok))
		expectStatus(t, w, http.StatusCreated)
		tag := decode[tagDto](t, w)
		taskID := e.scalar(`SELECT "Id" FROM "ReviewTasks" WHERE "TargetType" = 1 AND "TargetId" = $1 AND "Kind" = 0`, tag.ID)

		var wg sync.WaitGroup
		codes := make(chan int, 2)
		wg.Add(2)
		go func() {
			defer wg.Done()
			codes <- e.do("PATCH", fmt.Sprintf("/review/tasks/%d/approve", taskID), nil, withToken(modTok)).Code
		}()
		go func() {
			defer wg.Done()
			codes <- e.do("PATCH", fmt.Sprintf("/tags/%d", tag.ID), map[string]string{
				"name": fmt.Sprintf("Race %d edited", round), "description": "edited", "reviewStatus": "Accepted",
			}, withToken(modTok)).Code
		}()
		wg.Wait()
		close(codes)
		for c := range codes {
			if c >= 500 {
				t.Fatalf("round %d: concurrent approve and edit failed with %d", round, c)
			}
		}
	}
}
