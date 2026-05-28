# Contributing

Thanks for considering a contribution.

## Development setup

### Backend

Use Python 3.10. Do not rely on system Python when a virtual environment is available.

```bash
cd backend
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
python -m app.main
```

In this deployment workspace, tests are normally run with:

```bash
bash scripts/test_py310.sh
```

### Frontend

```bash
cd frontend
npm install
npm run dev
```

### Docker

```bash
docker compose up -d --build
```

## Before opening a pull request

Run:

```bash
bash scripts/test_py310.sh
cd frontend && npm run build
```

Also check that no local runtime data or secrets are included:

- `.env`
- `*.db`, `*.sqlite`, `*.sqlite3`
- subscription URLs or tokens
- `frontend/node_modules/`
- `frontend/dist/`
- virtualenv directories
- cache directories

## Project conventions

- Keep `README.md` focused on usage and maintenance.
- Keep `project.md` focused on design, architecture, decisions, and future plans.
- Do not add long private rule lists or real subscription data to documentation.
- GEOSITE rules should be checked against the actual geosite data source before being added.
- If a GEOSITE name does not exist, prefer a valid equivalent or use `DOMAIN-SUFFIX`.

## Security-sensitive contributions

If a change touches subscription fetching, proxy handling, generated output, or config persistence, please also update tests or explain why tests are not applicable.
