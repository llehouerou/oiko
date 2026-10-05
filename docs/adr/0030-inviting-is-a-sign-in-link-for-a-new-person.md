# Inviting is a Sign-in link for a new Person; a Guest's access may end on its own

Someone joins Oiko as a Member or a Guest, often a non-technical one, and Oiko has no mail server. Oiko has no invitation of its own. An Admin creates the Person (Name, Access level) and then, after step-up (ADR 0025), creates a Sign-in link for them (ADR 0026). The Person exists before anyone signs in, so the same flow covers a Person who never signs in and is invited later. A Sign-in link that an Admin creates for another Person is valid for 24 hours rather than 15 minutes, because it is usually sent through a messenger and opened hours later. A Guest may have an end date, after which they can no longer sign in, while the Person stays.

## Considered Options

- **An Invitation that creates the Person when redeemed**: the invitee types their own Name, but it adds a second kind of link to secure, revoke and document, and it cannot invite a Person who already exists.
- **Keeping 15 minutes for every link**: fine in person, with a QR code on the Admin's phone, but a remote invite becomes a race against the clock.
- **7 days, or a lifetime the Admin picks**: kinder to a guest who reads messages late, but it leaves a bearer credential live in a chat history for a week, or puts a security choice in the Admin's hands on every invite.
- **24 hours for every link, the host command's included**: one rule fewer, but whoever runs `oiko sign-in-link` is almost always the one who opens the link.
- **Oiko sending the link through a Notification channel**: it puts a credential in a third-party bot's history and depends on per-person Notifications.
- **Removing a Guest when their end date passes**: what they did would lose its author, and a returning guest would come back as a new Person.
- **An end date for Members too**: a Member with an end date is a rare case, and Guests are the temporary ones. An Admin never has one, since the last Admin must remain.

## Consequences

- **Lifetimes.** A Sign-in link lasts 24 hours when a signed-in Admin creates it for another Person, whether it is an invite or a replacement for lost Passkeys. It lasts 15 minutes when a Person creates it for themselves, and when the host command prints it. This amends ADR 0024.
- **One pending link.** A Person has at most one unused Sign-in link. Creating a new one revokes the previous unused link, whoever created it. An Admin sees a Person's pending link (who created it, when it expires) and can revoke it.
- **Delivery.** Oiko shows the link as a QR code, with a Copy button and the device's share sheet. It never sends the link itself.
- **First visit.** Opening the link shows who the invitee is about to become ("You've been invited to Oiko as <Name>", with the end date for a Guest) and a Continue button that signs them in. The Passkey offer (ADR 0024) follows in plain words, and declining it says that a new link will be needed once this Session ends. An expired or used link says so and tells the visitor to ask whoever sent it, without naming anyone.
- **End date.** Only a Guest may have one. An Admin sets, changes or removes it after step-up, and promoting the Guest to Member clears it. When it passes, the Guest's Sessions end and their event streams close (ADR 0025). Sign-in is then refused with a page saying the access has ended, and no Sign-in link can be created for them. Their Passkeys are kept, so setting a new end date lets the same guest back in.
- **Stored data.** The end date and the pending link are data Oiko owns, migrated under ADR 0019.
