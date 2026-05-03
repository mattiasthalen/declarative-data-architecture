package dab_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prism-data/prism/internal/contracts"
	"github.com/prism-data/prism/internal/dab"
)

func TestBuildPlan_FromCustomerFixture(t *testing.T) {
	f, err := contracts.LoadFocal(filepath.Join("..", "..", "testdata", "contracts", "valid", "dab", "customer.yml"))
	require.NoError(t, err)
	require.NoError(t, contracts.ValidateFocal(f))

	plan, err := dab.BuildEntityPlan(&contracts.FocalBundle{
		EntityID: "customer", Path: "ignored", Focal: f,
	})
	require.NoError(t, err)
	require.Equal(t, "customer", plan.Entity)

	// One DDL per table type:
	require.NotZero(t, plan.IDFR)
	require.NotZero(t, plan.Focal)
	require.NotZero(t, plan.Descriptor)
	require.Empty(t, plan.Relationships) // none in fixture

	// One mapping plan per (mapping_group, table) pair.
	require.Len(t, plan.Mappings, 1)
	m := plan.Mappings[0]
	require.Equal(t, "adventure_works", m.MappingGroup)
	require.Equal(t, "adventure_works", m.SourceID)
	require.Equal(t, "customer", m.SourceEntity)
	require.Equal(t, "current", m.From)
	require.Contains(t, m.IDFRExpr, "'CUSTOMER:'")
	require.Contains(t, m.EffTmstpExpr, "modified_date")

	// Descriptor mappings: one per outer attribute (CUSTOMER_NAME single-type;
	// CUSTOMER_LIFETIME_VALUE atomic group with two members).
	require.Len(t, m.Descriptors, 2)
	byAttr := map[string]dab.DescriptorMapping{}
	for _, d := range m.Descriptors {
		byAttr[d.AttrID] = d
	}
	name := byAttr["CUSTOMER_NAME"]
	require.Equal(t, "company_name", name.ValStrExpr)
	require.Empty(t, name.ValNumExpr)

	clv := byAttr["CUSTOMER_LIFETIME_VALUE"]
	require.Equal(t, "lifetime_value", clv.ValNumExpr)
	require.Equal(t, "'USD'", clv.UomExpr)
	require.Empty(t, clv.ValStrExpr)

	// Per-group typed view inputs.
	require.Len(t, plan.GroupViews, 2)
	require.Len(t, plan.EntityCurrentView.Attributes, 2)
}
