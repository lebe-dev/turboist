CREATE TABLE IF NOT EXISTS troiki_capacity (
    section_class TEXT NOT NULL PRIMARY KEY,  -- 'medium' or 'rest'
    capacity INTEGER NOT NULL DEFAULT 0       -- earned capacity, only grows
);
