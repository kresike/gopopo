gopopo is a policy server for the Postfix mailer written in GO.

It does very simple rate limiting of emails based on sasl username or
sender address where sasl is not available.
Rate limiting is done using in memory data, persistence is not implemented (yet?).

The algorithm used is a simple sliding window, the granularity is 1 minute and the
window length can be set using the configuration file, as ell as the default limit
for one sender.

Exceptions can be set in the whitelist file in the form of a postfix map file, which
has two columns. In the first column a user, domain or a sender can be specified, the
second column should contain OK, but it is not used at the moment.

If you have domains that need different limits these can be specified in the same manner
as the whitelist, with the exception that the first column can only contain domains.
The second column in this file contains the rate limit for that domain.

These map files do not support comments at this time.

Configuring the postfix mail server is very simple and can be done in two different ways.
The first one is by specifying the policy server in the `smtpd_recipient_restrictions`
directive, like this:

`smtpd_recipient_restrictions=check_policy_service inet:127.0.0.1:27091`

This will have the effect of counting every email as 1 message because the postfix server
does not know how many recipients the email message has at this stage.

The second and recommended way is to specify the policy server in the `smtpd_data_restrictions`
directive, like this:

`smtpd_data_restrictions=check_policy_service inet:127.0.0.1:27091`

This will count the emails by looking at the recipients, and counting every recipient.
This is recommended, because it prevents users from abusing the server by sending emails to
multiple recipients.
