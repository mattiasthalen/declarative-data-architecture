package contracts

import (
	"fmt"
	"strings"
)

// ValidateFocal runs JSON-Schema validation followed by cross-field rules
// (atomic-group completeness, mapping coverage, EFF_TMSTP agreement, etc.).
func ValidateFocal(f *Focal) error {
	v, err := toJSONValue(f)
	if err != nil {
		return err
	}
	if err := focalSchema.Validate(v); err != nil {
		return fmt.Errorf("focal schema: %w", err)
	}
	// Attribute ID uniqueness; group inner-ID uniqueness; group type uniqueness.
	seenAttr := map[string]bool{}
	groupMembers := map[string]map[string]string{} // outerID -> innerID -> type
	for _, a := range f.Attributes {
		if seenAttr[a.ID] {
			return fmt.Errorf("duplicate attribute id %q", a.ID)
		}
		seenAttr[a.ID] = true
		if len(a.Group) > 0 {
			members := map[string]string{}
			seenType := map[string]bool{}
			for _, m := range a.Group {
				if _, ok := members[m.ID]; ok {
					return fmt.Errorf("attribute %q: duplicate inner id %q", a.ID, m.ID)
				}
				if seenType[m.Type] {
					return fmt.Errorf("attribute %q: duplicate type %q in group", a.ID, m.Type)
				}
				members[m.ID] = m.Type
				seenType[m.Type] = true
			}
			groupMembers[a.ID] = members
		} else {
			// Single-type: synthesize a one-member group keyed by the outer ID.
			groupMembers[a.ID] = map[string]string{a.ID: a.Type}
		}
	}
	// Relationship ID uniqueness.
	seenRel := map[string]bool{}
	for _, r := range f.Relationships {
		if seenRel[r.ID] {
			return fmt.Errorf("duplicate relationship id %q", r.ID)
		}
		seenRel[r.ID] = true
	}
	// Mapping group rules.
	for _, mg := range f.MappingGroups {
		if mg.AllowMultipleIdentifiers {
			return fmt.Errorf("mapping group %q: allow_multiple_identifiers: true is not supported in M2", mg.Name)
		}
		for ti, t := range mg.Tables {
			if err := validateTable(f, mg.Name, ti, t, groupMembers, seenRel); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateTable enforces per-table cross-field rules.
func validateTable(
	f *Focal,
	groupName string, tableIdx int, t FocalMappingTable,
	groupMembers map[string]map[string]string,
	seenRel map[string]bool,
) error {
	bound := map[string]map[string]string{}    // outerID -> innerID -> bound (anything truthy)
	effExprs := map[string]map[string]string{} // outerID -> innerID -> per-attr eff_tmstp
	for ai, a := range t.Attributes {
		outer, inner, hasDot := strings.Cut(a.ID, ".")
		if !hasDot {
			inner = outer // single-type alias
		}
		members, ok := groupMembers[outer]
		if !ok {
			return fmt.Errorf("mapping_groups[%s].tables[%d].attributes[%d]: unknown attribute id %q", groupName, tableIdx, ai, a.ID)
		}
		if _, ok := members[inner]; !ok {
			return fmt.Errorf("mapping_groups[%s].tables[%d].attributes[%d]: unknown inner id %q for attribute %q", groupName, tableIdx, ai, inner, outer)
		}
		if bound[outer] == nil {
			bound[outer] = map[string]string{}
			effExprs[outer] = map[string]string{}
		}
		bound[outer][inner] = "y"
		if a.AttributeEffectiveTimestampExpression != "" {
			effExprs[outer][inner] = a.AttributeEffectiveTimestampExpression
		}
	}
	// Atomic-group completeness: for every bound outer, all members must be bound.
	for outer, members := range bound {
		want := groupMembers[outer]
		for innerID := range want {
			if _, ok := members[innerID]; !ok {
				return fmt.Errorf("mapping_groups[%s].tables[%d]: partial group binding of %q — inner id %q is unbound", groupName, tableIdx, outer, innerID)
			}
		}
		// EFF_TMSTP agreement
		var seen string
		for _, expr := range effExprs[outer] {
			if seen == "" {
				seen = expr
				continue
			}
			if expr != seen {
				return fmt.Errorf("mapping_groups[%s].tables[%d]: attribute %q has inconsistent attribute_effective_timestamp_expression across inner members", groupName, tableIdx, outer)
			}
		}
	}
	// Relationships: each bound id must exist in the model relationships.
	for ri, r := range t.Relationships {
		if !seenRel[r.ID] {
			return fmt.Errorf("mapping_groups[%s].tables[%d].relationships[%d]: relationship id %q not declared at model level", groupName, tableIdx, ri, r.ID)
		}
	}
	return nil
}
