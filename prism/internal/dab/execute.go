package dab

import (
	"context"
	"fmt"
	"strings"

	"github.com/prism-data/prism/internal/engine"
)

// DASSchemaPrefix is the prefix used by M1 for DAS staging schemas.
const DASSchemaPrefix = "das__"

// Execute runs an EntityPlan against an engine: ensures schema, creates DDL,
// runs all merges in plan-order, recomputes ROW_ST, then renders views.
func Execute(ctx context.Context, eng engine.Engine, plan *EntityPlan) error {
	d := eng.Dialect()

	if err := eng.Exec(ctx, d.CreateSchemaIfNotExists(plan.IDFR.Schema)); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	if err := eng.Exec(ctx, d.CreateIdfrTableIfNotExists(engine.IdfrTableSpec{
		Schema: plan.IDFR.Schema, Entity: plan.Entity,
	})); err != nil {
		return fmt.Errorf("create idfr: %w", err)
	}
	if err := eng.Exec(ctx, d.CreateFocalTableIfNotExists(engine.FocalTableSpec{
		Schema: plan.Focal.Schema, Entity: plan.Entity,
	})); err != nil {
		return fmt.Errorf("create focal: %w", err)
	}
	if err := eng.Exec(ctx, d.CreateDescriptorTableIfNotExists(engine.DescriptorTableSpec{
		Schema: plan.Descriptor.Schema, Entity: plan.Entity,
	})); err != nil {
		return fmt.Errorf("create descriptor: %w", err)
	}
	for _, r := range plan.Relationships {
		if err := eng.Exec(ctx, d.CreateRelationshipTableIfNotExists(engine.RelationshipTableSpec{
			Schema: r.Schema, Entity: r.Entity, Related: r.Related, Suffix: r.Suffix,
		})); err != nil {
			return fmt.Errorf("create relationship %s__%s: %w", r.Entity, r.Related, err)
		}
	}

	// Per-mapping merges.
	for _, m := range plan.Mappings {
		dasTable := fmt.Sprintf(`"%s%s"."%s__%s"`, DASSchemaPrefix, m.SourceID, m.SourceEntity, m.From)
		if err := mergeIDFRForMapping(ctx, eng, plan, m, dasTable); err != nil {
			return err
		}
		for _, dm := range m.Descriptors {
			if err := mergeDescriptorForMapping(ctx, eng, plan, m, dm, dasTable); err != nil {
				return err
			}
		}
		for _, rm := range m.Relationships {
			if err := mergeRelationshipForMapping(ctx, eng, plan, m, rm, dasTable); err != nil {
				return err
			}
		}
	}

	// Focal merge AFTER all IDFR rows are in.
	if err := eng.Exec(ctx, d.MergeFocal(engine.MergeFocalSpec{
		Schema: plan.Focal.Schema, Entity: plan.Entity,
	})); err != nil {
		return fmt.Errorf("merge focal: %w", err)
	}

	// Recompute ROW_ST / SEQ_NBR.
	if err := eng.Exec(ctx, d.RecomputeIdfrRowSt(engine.RecomputeIdfrRowStSpec{
		Schema: plan.IDFR.Schema, Entity: plan.Entity,
	})); err != nil {
		return fmt.Errorf("recompute idfr row_st: %w", err)
	}
	if err := eng.Exec(ctx, d.RecomputeDescriptorRowSt(engine.RecomputeDescriptorRowStSpec{
		Schema: plan.Descriptor.Schema, Entity: plan.Entity,
	})); err != nil {
		return fmt.Errorf("recompute descriptor row_st: %w", err)
	}
	for _, r := range plan.Relationships {
		if err := eng.Exec(ctx, d.RecomputeRelationshipRowSt(engine.RecomputeRelationshipRowStSpec{
			Schema: r.Schema, Entity: r.Entity, Related: r.Related, Suffix: r.Suffix,
		})); err != nil {
			return fmt.Errorf("recompute relationship row_st: %w", err)
		}
	}

	// Views.
	for _, gv := range plan.GroupViews {
		members := make([]engine.GroupViewMember, len(gv.Members))
		for i, m := range gv.Members {
			members[i] = engine.GroupViewMember{InnerID: m.InnerID, Type: m.Type}
		}
		if err := eng.Exec(ctx, d.CreateOrReplaceGroupView(engine.GroupViewSpec{
			Schema: gv.Schema, Entity: gv.Entity, AttrID: gv.AttrID,
			TypeKeyHex: gv.TypeKeyHex, Members: members,
		})); err != nil {
			return fmt.Errorf("create group view %s__%s: %w", gv.Entity, gv.AttrID, err)
		}
	}
	cv := plan.EntityCurrentView
	attrs := make([]engine.EntityCurrentAttribute, len(cv.Attributes))
	for i, a := range cv.Attributes {
		ms := make([]engine.GroupViewMember, len(a.Members))
		for j, m := range a.Members {
			ms[j] = engine.GroupViewMember{InnerID: m.InnerID, Type: m.Type}
		}
		attrs[i] = engine.EntityCurrentAttribute{AttrID: a.AttrID, Members: ms}
	}
	if err := eng.Exec(ctx, d.CreateOrReplaceEntityCurrentView(engine.EntityCurrentViewSpec{
		Schema: cv.Schema, Entity: cv.Entity, Attributes: attrs,
	})); err != nil {
		return fmt.Errorf("create entity current view: %w", err)
	}
	return nil
}

func mergeIDFRForMapping(ctx context.Context, eng engine.Engine, plan *EntityPlan, m MappingPlan, dasTable string) error {
	whereClause := ""
	if m.Where != "" {
		whereClause = "    WHERE " + m.Where + "\n"
	}
	cte := fmt.Sprintf(
		"    SELECT\n        %s AS \"%s_idfr\",\n        CAST((%s) AS TIMESTAMP) AS eff_tmstp\n    FROM %s\n%s",
		m.IDFRExpr, plan.Entity, m.EffTmstpExpr, dasTable, whereClause,
	)
	return eng.Exec(ctx, eng.Dialect().MergeIdfr(engine.MergeIdfrSpec{
		Schema: plan.IDFR.Schema, Entity: plan.Entity,
		MappingGroup: m.MappingGroup, InstRowKey: m.InstRowKey,
		SourceCTE: strings.TrimRight(cte, "\n"),
	}))
}

func mergeDescriptorForMapping(ctx context.Context, eng engine.Engine, plan *EntityPlan, m MappingPlan, dm DescriptorMapping, dasTable string) error {
	wheres := []string{}
	if m.Where != "" {
		wheres = append(wheres, "("+m.Where+")")
	}
	if dm.Where != "" {
		wheres = append(wheres, "("+dm.Where+")")
	}
	whereClause := ""
	if len(wheres) > 0 {
		whereClause = "    WHERE " + strings.Join(wheres, " AND ") + "\n"
	}
	cte := fmt.Sprintf(
		"    SELECT\n"+
			"        md5(%s) AS \"%s_key\",\n"+
			"        '%s' AS type_key,\n"+
			"        CAST((%s) AS TIMESTAMP) AS eff_tmstp,\n"+
			"        %s AS val_str,\n"+
			"        %s AS val_num,\n"+
			"        %s AS uom,\n"+
			"        %s AS sta_tmstp,\n"+
			"        %s AS end_tmstp\n"+
			"    FROM %s\n%s",
		m.IDFRExpr, plan.Entity,
		dm.TypeKeyHex,
		dm.EffTmstpExpr,
		nullIfEmpty(dm.ValStrExpr, "VARCHAR"),
		nullIfEmpty(dm.ValNumExpr, "DOUBLE"),
		nullIfEmpty(dm.UomExpr, "VARCHAR"),
		nullIfEmpty(dm.StaTmstpExpr, "TIMESTAMP"),
		nullIfEmpty(dm.EndTmstpExpr, "TIMESTAMP"),
		dasTable, whereClause,
	)
	return eng.Exec(ctx, eng.Dialect().MergeDescriptor(engine.MergeDescriptorSpec{
		Schema: plan.Descriptor.Schema, Entity: plan.Entity,
		MappingGroup: m.MappingGroup, InstRowKey: m.InstRowKey,
		SourceCTE: strings.TrimRight(cte, "\n"),
	}))
}

func mergeRelationshipForMapping(ctx context.Context, eng engine.Engine, plan *EntityPlan, m MappingPlan, rm RelationshipMapping, dasTable string) error {
	wheres := []string{}
	if m.Where != "" {
		wheres = append(wheres, "("+m.Where+")")
	}
	if rm.Where != "" {
		wheres = append(wheres, "("+rm.Where+")")
	}
	whereClause := ""
	if len(wheres) > 0 {
		whereClause = "    WHERE " + strings.Join(wheres, " AND ") + "\n"
	}
	cte := fmt.Sprintf(
		"    SELECT\n"+
			"        md5(%s) AS \"%s_key\",\n"+
			"        md5(CAST((%s) AS VARCHAR)) AS \"%s_key\",\n"+
			"        '%s' AS type_key,\n"+
			"        CAST((%s) AS TIMESTAMP) AS eff_tmstp\n"+
			"    FROM %s\n%s",
		m.IDFRExpr, plan.Entity,
		rm.TargetExpr, rm.Related,
		rm.TypeKeyHex,
		rm.EffTmstpExpr,
		dasTable, whereClause,
	)
	return eng.Exec(ctx, eng.Dialect().MergeRelationship(engine.MergeRelationshipSpec{
		Schema: plan.IDFR.Schema, Entity: plan.Entity,
		Related: rm.Related, Suffix: rm.Suffix,
		MappingGroup: m.MappingGroup, InstRowKey: m.InstRowKey,
		SourceCTE: strings.TrimRight(cte, "\n"),
	}))
}

// nullIfEmpty returns "NULL" cast to t when expr is empty, otherwise the expr
// cast to t. Empty expressions arise when a group doesn't bind a particular
// type slot (e.g. a STRING-only group leaves val_num/uom/sta/end as NULL).
func nullIfEmpty(expr, t string) string {
	if expr == "" {
		return "CAST(NULL AS " + t + ")"
	}
	return "CAST((" + expr + ") AS " + t + ")"
}
