package eve

import (
	"context"
	"path/filepath"
	"testing"
)

func TestBundledSDEProvidesChineseNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sde", "eve-sde.sqlite")
	if err := EnsureBundledSDE(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	idx := NewSDEIndex("")
	defer idx.Close()
	st, err := idx.OpenDatabase(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Version != BundledSDEVersion {
		t.Fatalf("version=%q", st.Version)
	}
	names := idx.TypeNames([]int64{58966, 46381, 7125, 11855})
	for id, name := range map[int64]string{58966: "紧凑型拦截失效装置", 46381: "星际代理处后勤包裹", 7125: "超级无焦点激光器I", 11855: "蛋白食物"} {
		if names[id] != name {
			t.Errorf("type %d=%q want %q", id, names[id], name)
		}
	}
	regions := idx.Regions()
	found := false
	for _, r := range regions {
		if r.ID == "10000016" && r.Name == "长征" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Chinese region 长征 missing")
	}
}
