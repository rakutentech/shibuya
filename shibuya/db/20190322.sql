use shibuya;

ALTER TABLE collection_plan DROP proxy;

ALTER TABLE collection_plan add csv_split tinyint(1) DEFAULT 0;