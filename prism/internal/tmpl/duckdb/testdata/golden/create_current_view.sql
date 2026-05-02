CREATE OR REPLACE VIEW "das__adventure_works"."customer__current" AS
SELECT * EXCLUDE (_record_hash, _dlt_id, _dlt_load_id, _loaded_at, _row_num)
FROM (
    SELECT *,
        ROW_NUMBER() OVER (
            PARTITION BY "customer_id"
            ORDER BY _loaded_at DESC, _record_hash DESC
        ) AS _row_num
    FROM "das__adventure_works"."customer__historized"
)
WHERE _row_num = 1;
