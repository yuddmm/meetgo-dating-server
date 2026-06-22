-- 000008_likes_chats (up)
-- Likes (MeetingCandidate) and minimal chat entity.

CREATE TABLE meeting_candidates (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    meeting_ad_id UUID        NOT NULL REFERENCES meeting_ads (id) ON DELETE CASCADE,
    profile_id    UUID        NOT NULL REFERENCES profiles (id) ON DELETE CASCADE,
    status        TEXT        NOT NULL DEFAULT 'PENDING'
                      CHECK (status IN ('PENDING', 'ACCEPTED', 'REJECTED')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- one response per (ad, profile)
    UNIQUE (meeting_ad_id, profile_id)
);

CREATE INDEX meeting_candidates_ad_status_idx ON meeting_candidates (meeting_ad_id, status);

-- Minimal chat: opens on an accepted candidate (1:1). Timer/freeze/messages later.
CREATE TABLE chats (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    meeting_candidate_id UUID        NOT NULL UNIQUE REFERENCES meeting_candidates (id) ON DELETE CASCADE,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
