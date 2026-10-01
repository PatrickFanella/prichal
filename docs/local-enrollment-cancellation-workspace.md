# Browser queued cancellation review

Status: locally implemented and qualified on October 1, 2026. Screenshot, screen-reader and fresh native qualification remain open.

The first-account panel offers queued cancellation only when a fresh scoped response explicitly says `canCancel`. Review starts unchecked and focuses its heading. Closing it returns focus to the launcher. Acknowledgement explains that native preparation may win and cause refusal. The action does not delete an account or server data.

Before POST, the browser retains only the enrollment UUID in session storage. Storage failure prevents submission. An unconfirmed response hides invitation/replacement controls and requires a fresh exact-ticket read before another review. Reload never submits automatically. Unknown or denied tickets preserve the key and focus the access error. Conflicting cancellation/withdrawal keys block controls and require support review rather than choosing one action.

A confirmed cancelled result clears the transport key, focuses status and permits a separately authorized bounded recovery. A fresh response that says cancellation is no longer eligible also clears the key and focuses the current result without claiming cancellation succeeded. The browser never changes that request into invitation withdrawal automatically. Invalid/unavailable status removes controls. Mutations invalidate older reads and use bounded requests; scope cleanup prevents stale completion and pending result focus.

## Verification

T3 keyboard/DOM fixtures qualified unchecked review, heading focus, acknowledgement, lost response, exact-ticket retry, review reset and close-review focus return. An unavailable or denied scoped read removed controls while retaining the key. Confirmation offered recovery, which was not submitted. A dispatch-wins conflict removed cancellation without claiming success or offering replacement. Storage failure prevented another POST, and invalid eligibility removed controls and focused its error.

Review fit 320×740, 375×812, 844×390, 768×1024 and 1440×900 with no document/panel overflow or controls outside its bounds. Three cancellation POSTs and 73 reads were intercepted, with zero source mutations or other attempted mutations. Original fetch was restored at 22:18:05Z. A second invalid-status read after hiding unconfirmed prepare controls passed with zero mutations and had its override removed immediately.

Real reload at 22:19:22Z retained a nonexistent disposable ticket and focused the real access error without controls. Reload with two synthetic action keys at 22:19:27Z retained both and blocked all actions. Synthetic keys were removed and actual verified status restored at 22:20:02Z. A further real reload at 22:20:58Z used the known verified enrollment ticket, cleared its cancellation key and focused preserved account status without falsely confirming cancellation. No fixture or action key remains.

The initial production build caught a missing JSX closing brace, which was fixed. Browser review identified an unconfirmed prepare button still rendered disabled; the final version hides it until a successful status read. Result focus is committed through a guarded effect so resolving a retained key does not cancel its own focus. The final full smoke gate, including production build, passed after the focus adjustment.

Source management work and both native inventories, container start times, signed records/media and custody matched the baseline. Private boolean/counter evidence is in `.local/cancellation-workspace-browser.json`. Schema stays 29; retained tenants, native slots, exports and recovery copies are preserved. PR #91 commit `92a1b00` passed remote CI 11630; backend cancellation PR #92 commit `d10c055` passed remote CI 11634 and is linked.

## Remaining gates

T3 snapshots failed again; the automation connection briefly disappeared and was restored by preview reopen. Correcting the keyboard key from a whitespace string to `Space` allowed the keyboard exercise. Visual snapshots and screen-reader checks remain unqualified. Positive browser cancellation is exercised through intercepted disposable responses and database tests, not a fresh native source deployment. Empty-PDS/native withdrawal and independent public DNS, TLS, mail, federation and commercial hosting remain open.
