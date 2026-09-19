// Command sde-import converts CCP's official JSONL SDE zip into Alice-EVE's normalized JSONL or SQLite.
package main

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"eve-assistant/desktop-app/internal/evesde"
)

type localized map[string]string

func (l localized) best() string {
	if s := strings.TrimSpace(l["zh"]); s != "" {
		return s
	}
	return strings.TrimSpace(l["en"])
}

type out struct {
	Kind                string  `json:"kind"`
	ID                  int64   `json:"id,omitempty"`
	Name                string  `json:"name,omitempty"`
	Published           *bool   `json:"published,omitempty"`
	CategoryID          int64   `json:"category_id,omitempty"`
	GroupID             int64   `json:"group_id,omitempty"`
	MarketGroupID       *int64  `json:"market_group_id,omitempty"`
	Description         string  `json:"description,omitempty"`
	RegionID            int64   `json:"region_id,omitempty"`
	ConstellationID     int64   `json:"constellation_id,omitempty"`
	Security            float64 `json:"security,omitempty"`
	SystemID            int64   `json:"system_id,omitempty"`
	DestinationSystemID int64   `json:"destination_system_id,omitempty"`
	ParentID            *int64  `json:"parent_id,omitempty"`
	BlueprintTypeID     int64   `json:"blueprint_type_id,omitempty"`
	ProductTypeID       *int64  `json:"product_type_id,omitempty"`
	ProductionTime      int64   `json:"production_time,omitempty"`
	MaterialTypeID      int64   `json:"material_type_id,omitempty"`
	Quantity            int64   `json:"quantity,omitempty"`
	PackagedVolume      float64 `json:"packaged_volume,omitempty"`
	Volume              float64 `json:"volume,omitempty"`
	Capacity            float64 `json:"capacity,omitempty"`
}
type common struct {
	Key             int64     `json:"_key"`
	Name            localized `json:"name"`
	Description     localized `json:"description"`
	Published       *bool     `json:"published"`
	CategoryID      int64     `json:"categoryID"`
	GroupID         int64     `json:"groupID"`
	MarketGroupID   *int64    `json:"marketGroupID"`
	ParentGroupID   *int64    `json:"parentGroupID"`
	RegionID        int64     `json:"regionID"`
	ConstellationID int64     `json:"constellationID"`
	Security        float64   `json:"securityStatus"`
	SolarSystemID   int64     `json:"solarSystemID"`
	Destination     struct {
		SolarSystemID int64 `json:"solarSystemID"`
	} `json:"destination"`
	PackagedVolume float64 `json:"packagedVolume"`
	Volume         float64 `json:"volume"`
	Capacity       float64 `json:"capacity"`
}
type blueprint struct {
	BlueprintTypeID int64 `json:"blueprintTypeID"`
	Activities      struct {
		Manufacturing *struct {
			Time      int64                              `json:"time"`
			Materials []struct{ TypeID, Quantity int64 } `json:"materials"`
			Products  []struct{ TypeID, Quantity int64 } `json:"products"`
		} `json:"manufacturing"`
	} `json:"activities"`
}

func main() {
	zipPath := flag.String("zip", "", "official eve-sde-*-jsonl.zip")
	outPath := flag.String("out", "", "output .sqlite or .jsonl (default: user Alice-EVE/sde/eve-sde.sqlite)")
	version := flag.String("version", "3503375", "SDE version/build")
	flag.Parse()
	if *zipPath == "" {
		fmt.Fprintln(os.Stderr, "-zip is required")
		os.Exit(2)
	}
	if *outPath == "" {
		d, e := os.UserConfigDir()
		if e != nil {
			panic(e)
		}
		*outPath = filepath.Join(d, "Alice-EVE", "sde", "eve-sde.sqlite")
	}
	if err := run(*zipPath, *outPath, *version); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(zpath, target, version string) error {
	zr, err := zip.OpenReader(zpath)
	if err != nil {
		return err
	}
	defer zr.Close()
	tmp, err := os.CreateTemp("", "alice-eve-sde-*.jsonl")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	bw := bufio.NewWriterSize(tmp, 1<<20)
	enc := json.NewEncoder(bw)
	count := map[string]int{}
	emit := func(x out) error { count[x.Kind]++; return enc.Encode(x) }
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}
	for _, spec := range []struct{ file, kind string }{{"categories.jsonl", "category"}, {"groups.jsonl", "group"}, {"mapRegions.jsonl", "region"}, {"mapConstellations.jsonl", "constellation"}, {"mapSolarSystems.jsonl", "system"}, {"mapStargates.jsonl", "stargate"}} {
		if err = each(files[spec.file], func(c common) error {
			x := out{Kind: spec.kind, ID: c.Key, Name: c.Name.best(), Published: c.Published, CategoryID: c.CategoryID, RegionID: c.RegionID, ConstellationID: c.ConstellationID, Security: c.Security, SystemID: c.SolarSystemID, DestinationSystemID: c.Destination.SolarSystemID}
			return emit(x)
		}); err != nil {
			return err
		}
	}
	// Parent rows must precede children because SQLite checks self-referencing FKs immediately.
	var markets []common
	if err = each(files["marketGroups.jsonl"], func(c common) error { markets = append(markets, c); return nil }); err != nil {
		return err
	}
	sort.SliceStable(markets, func(i, j int) bool { return depth(markets[i], markets) < depth(markets[j], markets) })
	for _, c := range markets {
		if err = emit(out{Kind: "market_group", ID: c.Key, Name: c.Name.best(), Description: c.Description.best(), ParentID: c.ParentGroupID}); err != nil {
			return err
		}
	}
	if err = each(files["types.jsonl"], func(c common) error {
		return emit(out{Kind: "type", ID: c.Key, Name: c.Name.best(), Description: c.Description.best(), Published: c.Published, GroupID: c.GroupID, MarketGroupID: c.MarketGroupID, PackagedVolume: c.PackagedVolume, Volume: c.Volume, Capacity: c.Capacity})
	}); err != nil {
		return err
	}
	if err = eachBP(files["blueprints.jsonl"], func(b blueprint) error {
		m := b.Activities.Manufacturing
		if m == nil {
			return nil
		}
		var product *int64
		if len(m.Products) > 0 {
			v := m.Products[0].TypeID
			product = &v
		}
		if err := emit(out{Kind: "blueprint", BlueprintTypeID: b.BlueprintTypeID, ProductTypeID: product, ProductionTime: m.Time}); err != nil {
			return err
		}
		for _, a := range m.Materials {
			if err := emit(out{Kind: "material", BlueprintTypeID: b.BlueprintTypeID, MaterialTypeID: a.TypeID, Quantity: a.Quantity}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err = bw.Flush(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if strings.EqualFold(filepath.Ext(target), ".jsonl") {
		os.MkdirAll(filepath.Dir(target), 0700)
		return os.Rename(tmpName, target)
	}
	in, err := os.Open(tmpName)
	if err != nil {
		return err
	}
	defer in.Close()
	if err = evesde.Import(context.Background(), target, in, evesde.Manifest{Version: version, Source: filepath.Base(zpath)}, evesde.ImportOptions{MaxBytes: 2 << 30, MaxRecords: 4_000_000}); err != nil {
		return err
	}
	fmt.Printf("generated %s\ncounts: %v\n", target, count)
	return nil
}
func each(f *zip.File, fn func(common) error) error {
	if f == nil {
		return fmt.Errorf("required zip entry missing")
	}
	r, e := f.Open()
	if e != nil {
		return e
	}
	defer r.Close()
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 64<<10), 16<<20)
	for s.Scan() {
		var c common
		if e = json.Unmarshal(s.Bytes(), &c); e != nil {
			return e
		}
		if e = fn(c); e != nil {
			return e
		}
	}
	return s.Err()
}
func eachBP(f *zip.File, fn func(blueprint) error) error {
	if f == nil {
		return fmt.Errorf("blueprints.jsonl missing")
	}
	r, e := f.Open()
	if e != nil {
		return e
	}
	defer r.Close()
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 64<<10), 16<<20)
	for s.Scan() {
		var b blueprint
		if e = json.Unmarshal(s.Bytes(), &b); e != nil {
			return e
		}
		if e = fn(b); e != nil {
			return e
		}
	}
	return s.Err()
}
func depth(c common, all []common) int {
	p := c.ParentGroupID
	d := 0
	for p != nil && d < len(all) {
		d++
		var next *int64
		for i := range all {
			if all[i].Key == *p {
				next = all[i].ParentGroupID
				break
			}
		}
		p = next
	}
	return d
}
