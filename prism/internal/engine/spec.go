package engine

// Column describes one declared column in a DAS contract, after type parsing.
type Column struct {
	SourcePath string // e.g. "CustomerID" or "Address.City"
	TargetName string // e.g. "customer_id"
	SQLType    string // dialect-rendered, e.g. "BIGINT", "DECIMAL(18,4)", "VARCHAR"
	NotNull    bool   // true when contract mode is REQUIRED
}

type HistorizedTableSpec struct {
	Schema  string
	Name    string // e.g. "customer__historized"
	Columns []Column
}

type HistorizedAppendSpec struct {
	Schema      string
	Name        string // e.g. "customer__historized"
	LakeGlob    string // e.g. "/abs/path/_lake/das/adventure_works/Customer/**/*.jsonl.gz"
	Compression string // "gzip"
	Columns     []Column
}

type CurrentViewSpec struct {
	Schema          string
	Name            string   // e.g. "customer__current"
	HistorizedTable string   // e.g. "customer__historized"
	PrimaryKey      []string // target_name list
}

// --- M2 (DAB) specs ---------------------------------------------------------

// IdfrTableSpec describes the IDFR audit table for one focal entity.
type IdfrTableSpec struct {
	Schema string // "dab"
	Entity string // lower-snake; produces dab.<entity>__idfr
}

// FocalTableSpec describes the focal table (one row per surrogate key).
type FocalTableSpec struct {
	Schema string
	Entity string // lower-snake; produces dab.<entity>
}

// DescriptorTableSpec describes the generic-EAV descriptor table for a focal.
type DescriptorTableSpec struct {
	Schema string
	Entity string // lower-snake; produces dab.<entity>__descriptor
}

// RelationshipTableSpec describes one relationship table on a focal.
// Suffix is empty unless the same focal has multiple relationships to the same
// target, in which case Suffix is "_<rel_id_lower>".
type RelationshipTableSpec struct {
	Schema  string
	Entity  string // lower-snake source focal
	Related string // lower-snake target focal
	Suffix  string // e.g. "" or "_places_order_alt"
}

// MergeIdfrSpec is one IDFR insert from a (mapping_group, table) contribution.
// SourceCTE is a SELECT that produces, at minimum, columns:
//   <entity>_idfr  VARCHAR
//   eff_tmstp      TIMESTAMP
type MergeIdfrSpec struct {
	Schema       string
	Entity       string
	MappingGroup string // INST_KEY
	InstRowKey   string // INST_ROW_KEY ("<source>.<entity>")
	SourceCTE    string // SELECT expression body, no leading SELECT keyword required
}

// MergeFocalSpec inserts/upserts the focal row for every key in
// dab.<entity>__idfr, refreshing eff_tmstp = MIN(idfr.eff_tmstp).
type MergeFocalSpec struct {
	Schema string
	Entity string
}

// MergeDescriptorSpec inserts descriptor rows for one (mapping_group, table)
// + one outer attribute. SourceCTE columns:
//   <entity>_key   VARCHAR
//   type_key       VARCHAR
//   eff_tmstp      TIMESTAMP
//   val_str        VARCHAR
//   val_num        DOUBLE
//   uom            VARCHAR
//   sta_tmstp      TIMESTAMP
//   end_tmstp      TIMESTAMP
type MergeDescriptorSpec struct {
	Schema       string
	Entity       string
	MappingGroup string
	InstRowKey   string
	SourceCTE    string
}

// MergeRelationshipSpec inserts relationship rows. SourceCTE columns:
//   <entity>_key   VARCHAR
//   <related>_key  VARCHAR
//   type_key       VARCHAR
//   eff_tmstp      TIMESTAMP
type MergeRelationshipSpec struct {
	Schema       string
	Entity       string
	Related      string
	Suffix       string
	MappingGroup string
	InstRowKey   string
	SourceCTE    string
}

// RecomputeIdfrRowStSpec recomputes ROW_ST and SEQ_NBR over the full IDFR
// table. Partition: (entity_key, idfr); order: (eff_tmstp, ver_tmstp).
type RecomputeIdfrRowStSpec struct {
	Schema string
	Entity string
}

// RecomputeDescriptorRowStSpec recomputes over descriptor.
// Partition: (entity_key, type_key); order: (eff_tmstp, ver_tmstp).
type RecomputeDescriptorRowStSpec struct {
	Schema string
	Entity string
}

// RecomputeRelationshipRowStSpec recomputes over a relationship table.
// Partition: (entity_key, related_key, type_key).
type RecomputeRelationshipRowStSpec struct {
	Schema  string
	Entity  string
	Related string
	Suffix  string
}

// GroupViewSpec is one per-group typed view. The view exposes one column per
// member (named by InnerID, lower-snake), projected from the corresponding
// type slot in the descriptor table. Filtered to the matching TYPE_KEY hex.
type GroupViewSpec struct {
	Schema     string
	Entity     string
	AttrID     string // lower-snake (used in view name dab.<entity>__<attrid>)
	TypeKeyHex string // 32-char MD5 hex
	Members    []GroupViewMember
}

type GroupViewMember struct {
	InnerID string // lower-snake; column name in the view
	Type    string // STRING|NUMBER|UNIT|START_TIMESTAMP|END_TIMESTAMP
}

// EntityCurrentViewSpec is the per-entity __current view: focal joined with
// per-group views, ROW_ST='Y' only.
type EntityCurrentViewSpec struct {
	Schema     string
	Entity     string
	Attributes []EntityCurrentAttribute
}

type EntityCurrentAttribute struct {
	AttrID  string            // lower-snake outer
	Members []GroupViewMember // 1+ inner members
}
