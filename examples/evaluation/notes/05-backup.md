# Backing up the knowledge base

The backup command creates a complete SQLite snapshot containing notes, chunks, embeddings and source associations.
JSON export preserves note content but does not include source synchronization associations.
Back up before upgrading the database schema, because older programs cannot open a newer schema.
