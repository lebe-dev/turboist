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
