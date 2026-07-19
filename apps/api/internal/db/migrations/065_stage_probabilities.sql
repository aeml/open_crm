-- open-crm-deploy: expand
-- 0.6.2 Explainable stage probabilities for coherent forecasts.

ALTER TABLE deal_stages
    ADD COLUMN probability_percent SMALLINT DEFAULT 10
        CHECK (probability_percent BETWEEN 0 AND 100);

WITH positioned_stages AS (
    SELECT id,
           is_closed,
           is_won,
           position,
           MAX(position) FILTER (WHERE is_closed = FALSE)
               OVER (PARTITION BY organization_id, pipeline_id) AS max_open_position
    FROM deal_stages
)
UPDATE deal_stages stages
SET probability_percent = CASE
    WHEN positioned.is_closed AND positioned.is_won THEN 100
    WHEN positioned.is_closed THEN 0
    ELSE LEAST(
        90,
        GREATEST(
            10,
            ROUND(100.0 * positioned.position / NULLIF(positioned.max_open_position + 1, 0))::SMALLINT
        )
    )
END
FROM positioned_stages positioned
WHERE positioned.id = stages.id;

COMMENT ON COLUMN deal_stages.probability_percent IS
    'Admin-configured forecast probability. Closed-won stages are 100 and closed-lost stages are 0.';
