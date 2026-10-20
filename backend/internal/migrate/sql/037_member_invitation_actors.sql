-- Existing events have unknown actors; never infer an actor from current ownership.
ALTER TABLE member_invitation_events ADD COLUMN actor_id uuid REFERENCES owners(id);
