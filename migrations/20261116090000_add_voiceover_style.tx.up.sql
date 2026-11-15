-- How a voiceover is delivered (natural, curious, excited, playful, warm,
-- storyteller); only expressive models perform styles other than natural.
ALTER TABLE voiceovers ADD COLUMN style text NOT NULL DEFAULT 'natural';
