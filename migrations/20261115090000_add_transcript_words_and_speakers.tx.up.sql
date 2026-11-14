-- Word times and speaker voices (transcription-v3).

-- Each segment's words as [{"s": start_ms, "e": end_ms, "t": text}, ...].
-- Null when the model's word times didn't hold up (captions estimate).
ALTER TABLE transcript_segments ADD COLUMN words jsonb;

-- How each speaker label's voice sounds: suggested by the model, and
-- correctable by people. It describes a voice, not who someone is.
CREATE TABLE transcript_speakers (
    transcript_id uuid    NOT NULL REFERENCES transcripts (id) ON DELETE CASCADE,
    position      integer NOT NULL,
    label         text    NOT NULL,
    voice         text    NOT NULL,
    voice_set_by  text    NOT NULL DEFAULT 'ai',
    PRIMARY KEY (transcript_id, label),
    CONSTRAINT transcript_speakers_voice_check CHECK (voice IN ('woman', 'man', 'unclear')),
    CONSTRAINT transcript_speakers_set_by_check CHECK (voice_set_by IN ('ai', 'person'))
);
