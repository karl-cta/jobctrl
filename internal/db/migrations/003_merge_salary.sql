ALTER TABLE applications ADD COLUMN salary INTEGER;
UPDATE applications SET salary = COALESCE(salary_min, salary_max);
ALTER TABLE applications DROP COLUMN salary_min;
ALTER TABLE applications DROP COLUMN salary_max;
