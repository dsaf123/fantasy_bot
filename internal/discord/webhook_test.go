package discord

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestSendOnlyAllowsUserMentions(t *testing.T) {
	var got payload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := NewClient(srv.URL).SendRich(context.Background(), "<@123> @everyone"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"users"}; !reflect.DeepEqual(got.AllowedMentions.Parse, want) {
		t.Errorf("allowed_mentions.parse = %v, want %v", got.AllowedMentions.Parse, want)
	}
}
