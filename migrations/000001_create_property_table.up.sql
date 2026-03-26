CREATE TABLE Property (
    id                  INT IDENTITY(1,1)  PRIMARY KEY,
    time_stamp          DATETIMEOFFSET     NOT NULL,
    unit_of_measurement NVARCHAR(MAX)      NOT NULL,
    value               NVARCHAR(MAX)      NOT NULL,
    identifier          NVARCHAR(255)      NOT NULL,
    slug                NVARCHAR(255)      NOT NULL
);

CREATE INDEX idx_properties_identifier ON Property (identifier);
CREATE INDEX idx_properties_timestamp  ON Property (time_stamp);
