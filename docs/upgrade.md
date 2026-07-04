# Upgrading Turboist

## Updating to a new version

Edit your `docker-compose.yml` and replace the image tag with the new version:

```yaml
image: tinyops/turboist:NEW_VERSION
```

Then pull the new image and restart the container:

```sh
docker compose pull
docker compose up -d
```

That's it. The app handles everything else on startup.

## Technical details

Turboist uses [Goose](https://github.com/pressly/goose) for database migrations. On every startup the app automatically applies any pending migrations to `data/turboist.db` before accepting traffic — no manual steps required.

If you want to be safe before upgrading, back up the database file first:

```sh
cp data/turboist.db data/turboist.db.bak
```

To restore from a backup, stop the container, replace the file, and start again:

```sh
docker compose down
cp data/turboist.db.bak data/turboist.db
docker compose up -d
```

## Rolling back to a previous version

Turboist does not ship down-migrations. If a new release added schema changes, rolling back the binary without restoring the database will likely cause errors.

The safe rollback sequence is:

1. Stop the container:
   ```sh
   docker compose down
   ```
2. Restore the pre-upgrade database backup:
   ```sh
   cp data/turboist.db.bak data/turboist.db
   ```
3. Set the old image tag in `docker-compose.yml`:
   ```yaml
   image: tinyops/turboist:PREVIOUS_VERSION
   ```
4. Start again:
   ```sh
   docker compose up -d
   ```

This is why taking a backup before every upgrade is strongly recommended.
