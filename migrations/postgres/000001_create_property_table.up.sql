CREATE TABLE IF NOT EXISTS Property (
        id 				SERIAL PRIMARY KEY,
        time_stamp 			TIMESTAMP WITH TIME ZONE NOT NULL,
        unit_of_measurement             TEXT NOT NULL,
        value				TEXT NOT NULL,
        identifier 			TEXT NOT NULL,
        slug 				TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_properties_identifier ON Property (identifier);
CREATE INDEX IF NOT EXISTS idx_properties_timestamp ON Property (time_stamp);