package main

import (
	"encoding/json"
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
