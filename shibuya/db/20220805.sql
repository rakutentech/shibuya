use shibuya;

CREATE TABLE IF NOT EXISTS collection_launch (
    id INT unsigned NOT NULL AUTO_INCREMENT PRIMARY KEY,
    collection_id INT unsigned NOT NULL UNIQUE
)CHARSET=utf8mb4;

ALTER TABLE collection_launch_history2 ADD COLUMN launch_id INT unsigned NOT NULL AFTER owner,
ADD INDEX (launch_id);