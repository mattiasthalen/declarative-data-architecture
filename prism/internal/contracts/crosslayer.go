package contracts

import "fmt"

// ValidateCrossLayer enforces references between DAS bundles and DAB focal
// bundles: every mapping_groups[].tables[].source and entity must resolve to a
// loaded DAS contract; every focal relationship's target_entity_id must
// resolve to a loaded focal entity.id.
func ValidateCrossLayer(das []*SourceBundle, dab []*FocalBundle) error {
	dasIdx := map[string]map[string]bool{}
	for _, b := range das {
		ents := map[string]bool{}
		for _, e := range b.Entities {
			ents[e.EntityID] = true
		}
		dasIdx[b.SourceID] = ents
	}
	focalIdx := map[string]bool{}
	for _, b := range dab {
		focalIdx[b.Focal.Entity.ID] = true
	}
	for _, b := range dab {
		f := b.Focal
		for _, mg := range f.MappingGroups {
			for ti, t := range mg.Tables {
				ents, ok := dasIdx[t.Source]
				if !ok {
					return fmt.Errorf("focal %s: mapping_groups[%s].tables[%d]: unknown DAS source %q", b.EntityID, mg.Name, ti, t.Source)
				}
				if !ents[t.Entity] {
					return fmt.Errorf("focal %s: mapping_groups[%s].tables[%d]: DAS source %q has no entity %q", b.EntityID, mg.Name, ti, t.Source, t.Entity)
				}
			}
		}
		for _, r := range f.Relationships {
			if !focalIdx[r.TargetEntityID] {
				return fmt.Errorf("focal %s: relationship %q: unknown target_entity_id %q (no focal declares this id)", b.EntityID, r.ID, r.TargetEntityID)
			}
		}
	}
	return nil
}
