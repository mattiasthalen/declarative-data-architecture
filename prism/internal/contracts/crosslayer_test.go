package contracts_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prism-data/prism/internal/contracts"
)

func TestCrossLayer_HappyPath(t *testing.T) {
	dasDir := filepath.Join("..", "..", "testdata", "contracts", "valid", "das")
	dabDir := filepath.Join("..", "..", "testdata", "contracts", "valid", "dab")
	dasBs, err := contracts.LoadAll(dasDir)
	require.NoError(t, err)
	dabBs, err := contracts.LoadAllDab(dabDir)
	require.NoError(t, err)
	require.NoError(t, contracts.ValidateCrossLayer(dasBs, dabBs))
}

func TestCrossLayer_RejectsUnknownSource(t *testing.T) {
	dasBs, err := contracts.LoadAll(filepath.Join("..", "..", "testdata", "contracts", "valid", "das"))
	require.NoError(t, err)
	bad := &contracts.FocalBundle{
		EntityID: "customer",
		Path:     "<test>",
		Focal: &contracts.Focal{
			Version: 1,
			Entity:  contracts.FocalIdent{ID: "CUSTOMER", Name: "CUSTOMER", Definition: "x"},
			Attributes: []contracts.FocalAttribute{{ID: "X", Definition: "x", Type: "STRING"}},
			MappingGroups: []contracts.FocalMappingGroup{{
				Name: "aw",
				Tables: []contracts.FocalMappingTable{{
					Source:      "no_such_source",
					Entity:      "customer",
					PrimaryKeys: []string{"customer_id"},
					Attributes:  []contracts.FocalMappingAttribute{{ID: "X", TransformationExpression: "company_name"}},
				}},
			}},
		},
	}
	err = contracts.ValidateCrossLayer(dasBs, []*contracts.FocalBundle{bad})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown DAS source")
}

func TestCrossLayer_RejectsUnknownTargetEntity(t *testing.T) {
	dasBs, err := contracts.LoadAll(filepath.Join("..", "..", "testdata", "contracts", "valid", "das"))
	require.NoError(t, err)
	bad := &contracts.FocalBundle{
		EntityID: "customer",
		Path:     "<test>",
		Focal: &contracts.Focal{
			Version: 1,
			Entity:  contracts.FocalIdent{ID: "CUSTOMER", Name: "CUSTOMER", Definition: "x"},
			Attributes: []contracts.FocalAttribute{{ID: "X", Definition: "x", Type: "STRING"}},
			Relationships: []contracts.FocalRelationship{
				{ID: "PLACES_GHOST", Definition: "x", TargetEntityID: "GHOST"},
			},
			MappingGroups: []contracts.FocalMappingGroup{{
				Name: "aw",
				Tables: []contracts.FocalMappingTable{{
					Source: "adventure_works", Entity: "customer",
					PrimaryKeys: []string{"customer_id"},
					Attributes:  []contracts.FocalMappingAttribute{{ID: "X", TransformationExpression: "company_name"}},
				}},
			}},
		},
	}
	err = contracts.ValidateCrossLayer(dasBs, []*contracts.FocalBundle{bad})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown target_entity_id")
}
