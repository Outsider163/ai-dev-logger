# SQLite lock troubleshooting

In this example project, a database write failed while another connection held a long write transaction.
The recorded fix was to shorten the transaction and avoid network requests inside it.
The team also configured a busy timeout so short lock waits could complete.
