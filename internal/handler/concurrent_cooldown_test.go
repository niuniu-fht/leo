package handler

import (
	"testing"
	"time"
)

type upstreamDetailErrStub struct {
	msg    string
	detail string
}

func (e *upstreamDetailErrStub) Error() string { return e.msg }
func (e *upstreamDetailErrStub) Detail() string {
	return e.detail
}

func TestIsConcurrentLimitError(t *testing.T) {
	if isConcurrentLimitError(nil) {
		t.Fatal("nil must not match")
	}
	plain := &upstreamDetailErrStub{msg: "image generate error: An error occurred.", detail: ""}
	if isConcurrentLimitError(plain) {
		t.Fatal("plain error must not match")
	}
	// marker only inside the captured upstream body (400-char truncated)
	detail := `{"data":null,"errors":[{"extensions":{"code":"BadRequestException","details":{"code":"VALIDATION_ERROR","errors":[{"code":"VALIDATION_ERROR","message":"You have reached the maximum number of concurrent GPT Image 2 requests (0). Please wait for your current gene`
	withDetail := &upstreamDetailErrStub{msg: "image generate error: An error occurred.", detail: detail}
	if !isConcurrentLimitError(withDetail) {
		t.Fatal("upstream detail carrying the concurrent marker must match")
	}
	direct := &upstreamDetailErrStub{msg: "maximum number of concurrent Nano Banana 2 requests (0)", detail: ""}
	if !isConcurrentLimitError(direct) {
		t.Fatal("message carrying the concurrent marker must match")
	}
}

func TestConcurrentCooldownExcludesFromScheduling(t *testing.T) {
	s := &Server{}
	s.ensureTokenDispatchBuckets()
	s.coolDownTokenDispatchBucket("tok-a", 2*time.Minute)
	if !s.isTokenDispatchCoolingDown("tok-a") {
		t.Fatal("token should be cooling down")
	}
	if s.isTokenDispatchCoolingDown("tok-b") {
		t.Fatal("other token should not be cooling down")
	}
}
