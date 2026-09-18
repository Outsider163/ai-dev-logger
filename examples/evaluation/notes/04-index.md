# Updating the knowledge index

After ingest --sync changes a note body, its old vectors are removed and its chunks are rebuilt.
Run embed --all to generate vectors for new or changed notes, then use status to check index completeness.
The sync command itself does not call the embedding service.
