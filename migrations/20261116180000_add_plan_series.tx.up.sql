-- Series: a plan run can ask for one story told in consecutive parts
-- (Part 1, Part 2, ...), each ending on a cliffhanger. Each plan then
-- records its part number and the series' title.
ALTER TABLE plan_runs ADD COLUMN series boolean NOT NULL DEFAULT false;
ALTER TABLE edit_plans
    ADD COLUMN part integer,
    ADD COLUMN series_title text NOT NULL DEFAULT '',
    ADD CONSTRAINT edit_plans_part_check CHECK (part IS NULL OR part BETWEEN 1 AND 10);
