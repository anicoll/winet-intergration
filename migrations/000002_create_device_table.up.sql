CREATE TABLE IF NOT EXISTS Device (
    id           	TEXT PRIMARY KEY,
    model        	TEXT,
    serial_number 	TEXT,
    created_at 		TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);