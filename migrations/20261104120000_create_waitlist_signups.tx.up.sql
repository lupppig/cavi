CREATE TABLE waitlist_signups (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    -- Address as submitted (trimmed), kept for correspondence.
    email            text        NOT NULL,
    -- Lower-cased address used for de-duplication.
    email_normalized text        NOT NULL,
    -- Optional self-described discipline, used to prioritise early access.
    discipline       text,
    created_at       timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT waitlist_signups_email_normalized_key UNIQUE (email_normalized),
    CONSTRAINT waitlist_signups_email_length CHECK (char_length(email) BETWEEN 3 AND 254),
    CONSTRAINT waitlist_signups_discipline_check CHECK (
        discipline IS NULL
        OR discipline IN ('clipper', 'creator', 'musician', 'studio', 'agency', 'brand', 'other')
    )
);
