-- name: UpsertDevice :exec
MERGE Device AS target
USING (SELECT @p1 AS id, @p2 AS model, @p3 AS serial_number) AS source
ON target.id = source.id
WHEN NOT MATCHED THEN
    INSERT (id, model, serial_number)
    VALUES (source.id, source.model, source.serial_number);
