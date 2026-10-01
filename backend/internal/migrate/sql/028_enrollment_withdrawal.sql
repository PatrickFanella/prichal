ALTER TABLE enrollment_operations ADD COLUMN withdrawal_requested_at timestamptz;
ALTER TABLE enrollment_operations ADD COLUMN withdrawal_attempts integer NOT NULL DEFAULT 0 CHECK(withdrawal_attempts BETWEEN 0 AND 3);
ALTER TABLE enrollment_operations DROP CONSTRAINT enrollment_operations_state_check;
ALTER TABLE enrollment_operations ADD CONSTRAINT enrollment_operations_state_check CHECK(state IN ('pending','invitation_ready','account_created','verified','expired','failed','superseded','withdrawing','withdrawn'));
ALTER TABLE enrollment_operations ADD CONSTRAINT enrollment_withdrawal_intent CHECK(
 (withdrawal_requested_at IS NULL AND withdrawal_attempts=0 AND state NOT IN ('withdrawing','withdrawn')) OR
 (withdrawal_requested_at IS NOT NULL AND withdrawal_attempts>0 AND state NOT IN ('pending','invitation_ready','expired') AND code_ciphertext IS NULL)
);
