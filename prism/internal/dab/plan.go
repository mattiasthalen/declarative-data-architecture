package dab

import (
	"fmt"
	"strings"

	"github.com/prism-data/prism/internal/contracts"
	"github.com/prism-data/prism/internal/naming"
)

// SchemaName is the single conformed schema where all DAB objects live.
const SchemaName = "dab"

// EntityPlan is the full set of operations needed to build one focal entity.
// Operations run in the order: DDLs → per-mapping merges → recompute → views.
type EntityPlan struct {
	Entity        string                 // lower-snake from filename (== focal id lower-cased)
	EntityID      string                 // upper-snake (focal.entity.id, used in TYPE_KEY hashing)
	IDFR          IDFRDDL                // create idfr table
	Focal         FocalDDL               // create focal table
	Descriptor    DescriptorDDL          // create descriptor table
	Relationships []RelationshipDDL      // one per declared relationship
	Mappings      []MappingPlan          // one per (mapping_group, table)
	GroupViews    []GroupViewPlan        // one per outer attribute
	EntityCurrentView EntityCurrentViewPlan
}

type IDFRDDL struct{ Schema, Entity string }
type FocalDDL struct{ Schema, Entity string }
type DescriptorDDL struct{ Schema, Entity string }

type RelationshipDDL struct {
	Schema       string
	Entity       string
	Related      string // lower-snake from focal.relationships[].target_entity_id
	Suffix       string // "" or "_<rel_id_lower>" for disambiguation
}

// MappingPlan describes one (mapping_group, table) contribution. The execute
// step builds a DAS-reading CTE from this plan and then emits idfr/focal/
// descriptor/relationship merges from it.
type MappingPlan struct {
	MappingGroup       string                  // INST_KEY value
	SourceID           string                  // DAS source id (lower-snake)
	SourceEntity       string                  // DAS entity id (lower-snake)
	From               string                  // "current" or "historized"
	InstRowKey         string                  // "<source>.<entity>"
	IDFRExpr           string                  // SQL: canonical IDFR string per row
	EffTmstpExpr       string                  // SQL: TIMESTAMP per row (table-level default)
	Where              string                  // SQL: optional table-level WHERE; empty when none
	Descriptors        []DescriptorMapping     // one per outer attribute bound by this table
	Relationships      []RelationshipMapping   // one per relationship bound by this table
}

type DescriptorMapping struct {
	AttrID         string // upper-snake outer attribute id (used for TYPE_KEY)
	TypeKeyHex     string // 32-char MD5
	EffTmstpExpr   string // SQL: per-group EFF_TMSTP (overrides MappingPlan.EffTmstpExpr if set)
	ValStrExpr     string // SQL or "" if not used in this group
	ValNumExpr     string
	UomExpr        string
	StaTmstpExpr   string
	EndTmstpExpr   string
	Where          string // combined per-attribute WHERE clauses (AND-joined); empty when none
}

type RelationshipMapping struct {
	RelID                string // upper-snake relationship id
	TypeKeyHex           string
	Related              string // lower-snake target focal id
	Suffix               string // "" or "_<rel_id_lower>"
	TargetExpr           string // SQL: target focal's canonical IDFR string
	EffTmstpExpr         string
	Where                string
}

// GroupViewPlan is the input for one per-group typed view.
type GroupViewPlan struct {
	Schema     string
	Entity     string
	AttrID     string // lower-snake (used in view name)
	TypeKeyHex string
	Members    []GroupViewMember
}

type GroupViewMember struct {
	InnerID string // lower-snake (column name in the view)
	Type    string // STRING|NUMBER|UNIT|START_TIMESTAMP|END_TIMESTAMP
}

// EntityCurrentViewPlan is the input for the per-entity __current view.
type EntityCurrentViewPlan struct {
	Schema     string
	Entity     string
	Attributes []EntityCurrentAttribute
}

type EntityCurrentAttribute struct {
	AttrID  string            // lower-snake outer
	Members []GroupViewMember // for atomic groups; for single-type, one member with InnerID == AttrID
}

// BuildEntityPlan compiles a parsed focal contract into an EntityPlan.
func BuildEntityPlan(b *contracts.FocalBundle) (*EntityPlan, error) {
	ent := b.EntityID
	entUp := b.Focal.Entity.ID

	plan := &EntityPlan{
		Entity:     ent,
		EntityID:   entUp,
		IDFR:       IDFRDDL{Schema: SchemaName, Entity: ent},
		Focal:      FocalDDL{Schema: SchemaName, Entity: ent},
		Descriptor: DescriptorDDL{Schema: SchemaName, Entity: ent},
		EntityCurrentView: EntityCurrentViewPlan{Schema: SchemaName, Entity: ent},
	}

	// Build a quick model index: outer attribute id -> list of (inner id, type).
	model := buildAttributeIndex(b.Focal.Attributes)

	// Group views + current view: one entry per outer attribute regardless of binding.
	for _, a := range b.Focal.Attributes {
		gv := GroupViewPlan{
			Schema:     SchemaName,
			Entity:     ent,
			AttrID:     strings.ToLower(a.ID),
			TypeKeyHex: TypeKeyHex(entUp, a.ID),
		}
		ec := EntityCurrentAttribute{AttrID: strings.ToLower(a.ID)}
		if len(a.Group) == 0 {
			gv.Members = []GroupViewMember{{InnerID: strings.ToLower(a.ID), Type: a.Type}}
			ec.Members = gv.Members
		} else {
			for _, m := range a.Group {
				gm := GroupViewMember{InnerID: strings.ToLower(m.ID), Type: m.Type}
				gv.Members = append(gv.Members, gm)
				ec.Members = append(ec.Members, gm)
			}
		}
		plan.GroupViews = append(plan.GroupViews, gv)
		plan.EntityCurrentView.Attributes = append(plan.EntityCurrentView.Attributes, ec)
	}

	// Relationships: one DDL per declared relationship; the suffix disambiguates
	// when one focal has multiple relationships to the same target.
	relCountByTarget := map[string]int{}
	for _, r := range b.Focal.Relationships {
		relCountByTarget[r.TargetEntityID]++
	}
	for _, r := range b.Focal.Relationships {
		related := strings.ToLower(r.TargetEntityID)
		suffix := ""
		if relCountByTarget[r.TargetEntityID] > 1 {
			suffix = "_" + strings.ToLower(r.ID)
		}
		plan.Relationships = append(plan.Relationships, RelationshipDDL{
			Schema: SchemaName, Entity: ent, Related: related, Suffix: suffix,
		})
	}

	// Mappings: one MappingPlan per (mapping_group, table).
	for _, mg := range b.Focal.MappingGroups {
		for _, t := range mg.Tables {
			mp, err := buildMappingPlan(entUp, mg.Name, t, model, b.Focal.Relationships, relCountByTarget)
			if err != nil {
				return nil, err
			}
			plan.Mappings = append(plan.Mappings, mp)
		}
	}
	return plan, nil
}

type modelIndex struct {
	outers map[string]struct {
		Attr    contracts.FocalAttribute
		Members map[string]string // inner id -> type (single-type uses one member with id=outer.id)
	}
}

func buildAttributeIndex(attrs []contracts.FocalAttribute) modelIndex {
	idx := modelIndex{outers: map[string]struct {
		Attr    contracts.FocalAttribute
		Members map[string]string
	}{}}
	for _, a := range attrs {
		members := map[string]string{}
		if len(a.Group) > 0 {
			for _, m := range a.Group {
				members[m.ID] = m.Type
			}
		} else {
			members[a.ID] = a.Type
		}
		idx.outers[a.ID] = struct {
			Attr    contracts.FocalAttribute
			Members map[string]string
		}{a, members}
	}
	return idx
}

func buildMappingPlan(
	entUp, groupName string,
	t contracts.FocalMappingTable,
	model modelIndex,
	rels []contracts.FocalRelationship,
	relCountByTarget map[string]int,
) (MappingPlan, error) {
	mp := MappingPlan{
		MappingGroup: groupName,
		SourceID:     t.Source,
		SourceEntity: t.Entity,
		From:         t.FromOrDefault(),
		InstRowKey:   t.Source + "." + t.Entity,
		IDFRExpr:     CanonicalIDFRExpr(t.PrimaryKeys),
		Where:        t.Where,
	}
	if t.EntityEffectiveTimestampExpression != "" {
		mp.EffTmstpExpr = t.EntityEffectiveTimestampExpression
	} else {
		mp.EffTmstpExpr = "_loaded_at"
	}

	// Group bindings: collect per outer attribute.
	type groupBinding struct {
		Outer            string
		Members          map[string]string // inner -> transformation_expression
		Where            []string          // per-inner where clauses
		EffTmstp         string            // per-group EFF_TMSTP override (or "")
	}
	bindings := map[string]*groupBinding{}
	for _, a := range t.Attributes {
		outer, inner, hasDot := strings.Cut(a.ID, ".")
		if !hasDot {
			inner = outer
		}
		if _, ok := model.outers[outer]; !ok {
			return mp, fmt.Errorf("mapping_groups[%s].tables[%s.%s].attributes: unknown attribute id %q", groupName, t.Source, t.Entity, a.ID)
		}
		gb := bindings[outer]
		if gb == nil {
			gb = &groupBinding{Outer: outer, Members: map[string]string{}}
			bindings[outer] = gb
		}
		gb.Members[inner] = a.TransformationExpression
		if a.Where != "" {
			gb.Where = append(gb.Where, a.Where)
		}
		if a.AttributeEffectiveTimestampExpression != "" {
			gb.EffTmstp = a.AttributeEffectiveTimestampExpression
		}
	}

	for _, gb := range bindings {
		entry := model.outers[gb.Outer]
		dm := DescriptorMapping{
			AttrID:     gb.Outer,
			TypeKeyHex: TypeKeyHex(entUp, gb.Outer),
		}
		if gb.EffTmstp != "" {
			dm.EffTmstpExpr = gb.EffTmstp
		} else {
			dm.EffTmstpExpr = mp.EffTmstpExpr
		}
		for innerID, typ := range entry.Members {
			expr, ok := gb.Members[innerID]
			if !ok {
				return mp, fmt.Errorf("internal: partial group %q (validate should have caught this)", gb.Outer)
			}
			switch typ {
			case "STRING":
				dm.ValStrExpr = expr
			case "NUMBER":
				dm.ValNumExpr = expr
			case "UNIT":
				dm.UomExpr = expr
			case "START_TIMESTAMP":
				dm.StaTmstpExpr = expr
			case "END_TIMESTAMP":
				dm.EndTmstpExpr = expr
			default:
				return mp, fmt.Errorf("attribute %s.%s: unknown type %q", gb.Outer, innerID, typ)
			}
		}
		if len(gb.Where) > 0 {
			dm.Where = "(" + strings.Join(gb.Where, ") AND (") + ")"
		}
		mp.Descriptors = append(mp.Descriptors, dm)
	}

	// Relationship bindings.
	relIdx := map[string]contracts.FocalRelationship{}
	for _, r := range rels {
		relIdx[r.ID] = r
	}
	for _, r := range t.Relationships {
		def, ok := relIdx[r.ID]
		if !ok {
			return mp, fmt.Errorf("mapping_groups[%s].tables[%s.%s].relationships: %q not declared", groupName, t.Source, t.Entity, r.ID)
		}
		related := strings.ToLower(def.TargetEntityID)
		suffix := ""
		if relCountByTarget[def.TargetEntityID] > 1 {
			suffix = "_" + strings.ToLower(r.ID)
		}
		mp.Relationships = append(mp.Relationships, RelationshipMapping{
			RelID:        r.ID,
			TypeKeyHex:   TypeKeyHex(entUp, r.ID),
			Related:      related,
			Suffix:       suffix,
			TargetExpr:   r.TargetTransformationExpression,
			EffTmstpExpr: mp.EffTmstpExpr,
			Where:        r.Where,
		})
	}
	_ = naming.ValidateSnakeCaseIdentifier // imported elsewhere; suppress unused if no further use here
	return mp, nil
}
