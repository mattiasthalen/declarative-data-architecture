// prism/internal/discover/odata.go
// Package discover fetches upstream metadata and scaffolds prism entity
// contracts. M1 supports OData $metadata; future providers add their own
// implementations. See spec section "CLI surface" / `prism das discover`.
package discover

import (
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/prism-data/prism/internal/naming"
)

type EntityType struct {
	Name       string
	Keys       []string
	Properties []Property
}

type Property struct {
	Name     string
	EDMType  string
	Nullable bool
}

// ParseODataMetadata parses an OData v4 $metadata XML document and returns
// one EntityType per declared <EntityType>.
func ParseODataMetadata(data []byte) ([]EntityType, error) {
	var doc struct {
		XMLName      xml.Name `xml:"Edmx"`
		DataServices struct {
			Schemas []struct {
				EntityTypes []struct {
					Name string `xml:"Name,attr"`
					Key  struct {
						PropertyRefs []struct {
							Name string `xml:"Name,attr"`
						} `xml:"PropertyRef"`
					} `xml:"Key"`
					Properties []struct {
						Name     string `xml:"Name,attr"`
						Type     string `xml:"Type,attr"`
						Nullable string `xml:"Nullable,attr"`
					} `xml:"Property"`
				} `xml:"EntityType"`
			} `xml:"Schema"`
		} `xml:"DataServices"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse $metadata: %w", err)
	}
	var out []EntityType
	for _, sch := range doc.DataServices.Schemas {
		for _, et := range sch.EntityTypes {
			ent := EntityType{Name: et.Name}
			for _, k := range et.Key.PropertyRefs {
				ent.Keys = append(ent.Keys, k.Name)
			}
			for _, p := range et.Properties {
				ent.Properties = append(ent.Properties, Property{
					Name: p.Name, EDMType: p.Type, Nullable: p.Nullable != "false",
				})
			}
			out = append(out, ent)
		}
	}
	return out, nil
}

var edmToPrism = map[string]string{
	"Edm.String":         "STRING",
	"Edm.Int16":          "INTEGER",
	"Edm.Int32":          "INTEGER",
	"Edm.Int64":          "BIGINT",
	"Edm.Boolean":        "BOOLEAN",
	"Edm.Date":           "DATE",
	"Edm.DateTimeOffset": "TIMESTAMP",
	"Edm.Guid":           "STRING",
	"Edm.Decimal":        "DECIMAL(38,9)",
}

// RenderEntityScaffold produces a draft entity contract YAML for ent.
func RenderEntityScaffold(ent EntityType) (string, error) {
	var b strings.Builder
	fmt.Fprintln(&b, "version: 1")
	fmt.Fprintln(&b, "entity:")
	fmt.Fprintf(&b, "  name: %s\n", ent.Name)
	fmt.Fprintln(&b, "schema:")
	fmt.Fprintln(&b, "  primary_key:")
	for _, k := range ent.Keys {
		fmt.Fprintf(&b, "    - %s\n", naming.ToSnakeCase(k))
	}
	fmt.Fprintln(&b, "  columns:")
	for _, p := range ent.Properties {
		typ, ok := edmToPrism[p.EDMType]
		if !ok {
			typ = "STRING" // safe fallback; user can refine
		}
		mode := "REQUIRED"
		if p.Nullable {
			mode = "NULLABLE"
		}
		fmt.Fprintf(&b, "    - source_path: %s\n", p.Name)
		fmt.Fprintf(&b, "      target_name: %s\n", naming.ToSnakeCase(p.Name))
		fmt.Fprintf(&b, "      type: %s\n", typ)
		fmt.Fprintf(&b, "      mode: %s\n", mode)
	}
	return b.String(), nil
}
