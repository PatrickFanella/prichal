ALTER TABLE enrollment_operations ADD COLUMN dispatch_started_at timestamptz;
-- Prior pending rows may have reached the agent before a lost checkpoint.
-- Treat every retained operation as dispatched; only new queued work is cancellable.
UPDATE enrollment_operations SET dispatch_started_at=updated_at;
ALTER TABLE enrollment_operations DROP CONSTRAINT enrollment_operations_state_check;
ALTER TABLE enrollment_operations ADD CONSTRAINT enrollment_operations_state_check CHECK(state IN ('pending','invitation_ready','account_created','verified','expired','failed','superseded','withdrawing','withdrawn','cancelled'));
ALTER TABLE enrollment_operations ADD CONSTRAINT enrollment_cancellation_checkpoint CHECK(state<>'cancelled' OR (dispatch_started_at IS NULL AND code_ciphertext IS NULL AND withdrawal_requested_at IS NULL AND account_did IS NULL AND account_handle IS NULL AND NOT email_confirmed AND expires_at IS NULL));
