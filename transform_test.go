package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/tidwall/gjson"
)

func TestOpenAIBlockUsesRuleOrderBeforeDocumentOrder(t *testing.T) {
	registerConfig(t, "mode: block\nwords: [ab, a]\n")
	body := []byte(`{"messages":[{"role":"user","content":"a"},{"role":"developer","content":"ab"}]}`)
	resp := interceptRPC(t, "openai", body)
	if !resp.Terminate || resp.StatusCode != 400 || len(resp.Body) != 0 {
		t.Fatalf("response = %#v", resp)
	}
	if string(resp.ResponseBody) != `{"error":{"type":"invalid_request_error","code":"censorship_blocked","message":"request blocked by censorship rule","term":"ab","role":"developer"}}` {
		t.Fatalf("response body = %s", resp.ResponseBody)
	}
}

func TestOpenAIBlockDocumentOrderAndErrorEscaping(t *testing.T) {
	registerConfig(t, "mode: block\nwords: [hit]\n")
	resp := interceptRPC(t, "openai", []byte(`{"messages":[{"role":"system","content":"hit"},{"role":"user","content":"hit"}]}`))
	if gjson.GetBytes(resp.ResponseBody, "error.role").String() != "system" {
		t.Fatalf("response body = %s", resp.ResponseBody)
	}

	term := "quote \" and\nnewline\n"
	registerConfig(t, "mode: block\nwords:\n  - |\n    quote \" and\n    newline\n")
	body, err := json.Marshal(map[string]any{"messages": []any{map[string]any{"role": "user", "content": term}}})
	if err != nil {
		t.Fatal(err)
	}
	resp = interceptRPC(t, "openai", body)
	if !json.Valid(resp.ResponseBody) || gjson.GetBytes(resp.ResponseBody, "error.term").String() != term {
		t.Fatalf("response body = %s", resp.ResponseBody)
	}
}

func assertBlockedRole(t *testing.T, sourceFormat, body, wantRole string) {
	t.Helper()
	registerConfig(t, "mode: block\nwords: [SECRET]\nscope:\n  roles: [system, developer, user, assistant, tool]\n")
	resp := interceptRPC(t, sourceFormat, []byte(body))
	if !resp.Terminate || resp.StatusCode != 400 || gjson.GetBytes(resp.ResponseBody, "error.term").String() != "SECRET" || gjson.GetBytes(resp.ResponseBody, "error.role").String() != wantRole {
		t.Fatalf("format=%s role=%s response=%#v body=%s", sourceFormat, wantRole, resp, resp.ResponseBody)
	}
}

func TestStripRemovesAllOccurrencesAcrossAllNodes(t *testing.T) {
	cfg := mustConfig(t, "mode: strip\nwords: [bad]\n")
	body := []byte(`{"messages":[{"role":"user","content":"bad bad"},{"role":"user","content":"xbadx"},{"role":"user","content":"bad/bad"}]}`)
	got, err := transformRequest(body, "openai", cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"messages":[{"role":"user","content":" "},{"role":"user","content":"xx"},{"role":"user","content":"/"}]}`
	if string(got.Body) != want {
		t.Fatalf("body = %s, want %s", got.Body, want)
	}
}

func TestStripOrderedCascadeAndFoldedOccurrences(t *testing.T) {
	cfg := mustConfig(t, "mode: strip\nignore_case: true\nwords: [AB, x]\n")
	body := []byte(`{"messages":[{"role":"user","content":"aBxABx"}]}`)
	got, _ := transformRequest(body, "openai", cfg)
	if string(got.Body) != `{"messages":[{"role":"user","content":""}]}` {
		t.Fatalf("body = %s", got.Body)
	}
}

func TestStripUsesLeftmostNonOverlappingOccurrences(t *testing.T) {
	cfg := mustConfig(t, "mode: strip\nwords: [aa]\n")
	body := []byte(`{"messages":[{"role":"user","content":"aaa"}]}`)
	got, err := transformRequest(body, "openai", cfg)
	if err != nil || string(got.Body) != `{"messages":[{"role":"user","content":"a"}]}` {
		t.Fatalf("body = %s, err = %v", got.Body, err)
	}
}

func TestObfsPreservesMatchedCaseAndInsertsOncePerOccurrence(t *testing.T) {
	cfg := mustConfig(t, "mode: obfs\nignore_case: true\nwords: [Alpha, 世界]\nobfs:\n  char: '⁠'\n")
	body := []byte(`{"messages":[{"role":"user","content":"ALPHA Alpha 世界世界"}]}`)
	got, err := transformRequest(body, "openai", cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"messages":[{"role":"user","content":"A⁠LPHA A⁠lpha 世⁠界世⁠界"}]}`
	if string(got.Body) != want {
		t.Fatalf("body = %s, want %s", got.Body, want)
	}
}

func TestRebuildMatchesDecodedEscapesAndUsesEncodingJSONEscaping(t *testing.T) {
	cfg := mustConfig(t, "mode: obfs\nwords: ['<X>']\n")
	u := string([]byte{0x5c, 'u'})
	encoded := u + "003cX" + u + "003e" + u + "0026" + u + "2028" + u + "2029"
	body := []byte(`{"messages":[{"role":"user","content":"` + encoded + `"}],"raw":"` + encoded + ` excluded"}`)
	got, err := transformRequest(body, "openai", cfg)
	if err != nil {
		t.Fatal(err)
	}
	wantToken := u + "003c" + "​" + "X" + u + "003e" + u + "0026" + u + "2028" + u + "2029"
	want := []byte(`{"messages":[{"role":"user","content":"` + wantToken + `"}],"raw":"` + encoded + ` excluded"}`)
	if !bytes.Equal(got.Body, want) {
		t.Fatalf("body = %q, want %q", got.Body, want)
	}
}

func TestTransformIsByteDeterministic(t *testing.T) {
	cfg := mustConfig(t, "mode: obfs\nignore_case: true\nwords: [Alpha]\n")
	body := []byte(" { \"messages\" : [ { \"role\" : \"user\", \"content\" : \"ALPHA Alpha\" } ], \"n\":1e+03 } ")
	want := []byte(" { \"messages\" : [ { \"role\" : \"user\", \"content\" : \"A" + "​" + "LPHA A" + "​" + "lpha\" } ], \"n\":1e+03 } ")
	var first []byte
	var firstHash [32]byte
	for i := 0; i < 100; i++ {
		got, err := transformRequest(body, "openai", cfg)
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(got.Body)
		if i == 0 {
			first = append([]byte(nil), got.Body...)
			firstHash = hash
			if !bytes.Equal(first, want) {
				t.Fatalf("span-external bytes changed: got %q, want %q", first, want)
			}
			continue
		}
		if !bytes.Equal(got.Body, first) || hash != firstHash {
			t.Fatalf("iteration %d was nondeterministic", i)
		}
	}

	const workers = 32
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				got, err := transformRequest(body, "openai", cfg)
				if err != nil {
					errs <- err
					return
				}
				if !bytes.Equal(got.Body, first) || sha256.Sum256(got.Body) != firstHash {
					errs <- fmt.Errorf("concurrent transform was nondeterministic")
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}
