use shibuya;

CREATE TABLE IF NOT EXISTS collection_data (
    filename varchar(191) NOT NULL,
    collection_id INT UNSIGNED NOT NULL,
    PRIMARY KEY(collection_id, filename)
)CHARSET=utf8mb4;