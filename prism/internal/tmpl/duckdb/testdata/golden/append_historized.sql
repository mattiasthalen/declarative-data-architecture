INSERT INTO "das__adventure_works"."customer__historized" (
    _record_hash, _dlt_id, _dlt_load_id, _loaded_at,
    "customer_id",
    "company_name",
    "modified_date"
)
SELECT
    md5(typed_row::VARCHAR)                 AS _record_hash,
    json_extract_string(json, '$._dlt_id')      AS _dlt_id,
    json_extract_string(json, '$._dlt_load_id') AS _dlt_load_id,
    CURRENT_TIMESTAMP                       AS _loaded_at,
    typed_row."customer_id",
    typed_row."company_name",
    typed_row."modified_date"
FROM (
    SELECT
        json,
        struct_pack(
            "customer_id" := CAST(json_extract(json, '$.CustomerID') AS BIGINT),
            "company_name" := CAST(json_extract(json, '$.CompanyName') AS VARCHAR),
            "modified_date" := CAST(json_extract(json, '$.ModifiedDate') AS TIMESTAMP)
        ) AS typed_row
    FROM read_ndjson_objects(
        '/lake/das/adventure_works/Customer/**/*.jsonl.gz',
        compression = 'gzip'
    ) AS t(json)
)
ON CONFLICT (_record_hash) DO NOTHING;
