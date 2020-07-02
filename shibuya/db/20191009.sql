use shibuya;
ALTER TABLE running_plan
ADD started_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
