package risk

import (
 "testing"
 "time"
)
func TestScoreDeterministicAndExpiry(t *testing.T) {
 now:=time.Unix(1000,0)
 events:=[]IntelEvent{{Source:"intel",Confidence:.8,Threat:"high",ExpiresAt:now.Add(time.Hour)},{Source:"old",Confidence:1,Threat:"critical",ExpiresAt:now.Add(-time.Second)}}
 if got:=Score(events,now); got!=60 { t.Fatalf("got %d",got) }
 if Score(events,now)!=Score(events,now) { t.Fatal("non deterministic") }
}
func TestInvalidConfidence(t *testing.T) { now:=time.Now(); if Score([]IntelEvent{{Source:"x",Confidence:2,Threat:"critical",ExpiresAt:now.Add(time.Hour)}},now)!=0 { t.Fatal("invalid event scored") } }
