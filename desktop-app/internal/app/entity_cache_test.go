package app
import("testing";"time")
func TestEntityCacheTTLAndBound(t *testing.T){c:=newEntityClientCache(15*time.Millisecond,2);c.put("a",1);if v,ok:=c.get("a");!ok||v.(int)!=1{t.Fatal("cache miss")};time.Sleep(20*time.Millisecond);if _,ok:=c.get("a");ok{t.Fatal("expired hit")};c.put("a",1);c.put("b",2);c.put("c",3);if len(c.items)>2{t.Fatalf("unbounded cache: %d",len(c.items))}}
