## Summary

<!-- What changed, and why? -->

## Type of change

- [ ] Feature
- [ ] Bug fix
- [ ] Database change
- [ ] Documentation
- [ ] Maintenance

## How it was tested

<!-- Commands run and what you checked by hand. Say what was not tested. -->

- [ ] `make lint test`
- [ ] `make test-db` (for API or database changes)
- [ ] Checked in the browser (for screen changes)

## Database

<!-- Delete this section if there is no schema change. -->

- [ ] New numbered `.up.sql` and `.down.sql` pair; no applied migration was edited
- [ ] New tables have row level security, a `dfipam_app_all` policy and least-privilege grants
- [ ] `make sql` was run

## Safety

- [ ] Every new route has a permission check
- [ ] Changes write an audit row in the same transaction
- [ ] Discovery and findings can't change allocation state or ownership
- [ ] Queries respect routing domains, and no address range is enumerated
- [ ] No secrets in code, logs or responses

## Documentation

- [ ] `docs/development_phase.md` is updated
- [ ] Other docs are updated, or no update is needed
