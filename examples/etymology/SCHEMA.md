# Etymology Notebook Schema

Spec for files under `notebooks/etymology/<book>/sessionN.yml` (and the
example under `examples/etymology/common-roots/origins.yml`).

A session file may carry three sections: `origins:` (required), and the
optional `concepts:` and `relations:` blocks added in this PR. Each origin
may also carry an optional `forms:` list. A worked example using neutral,
non-proprietary Latin roots lives next to this file
(`examples/etymology/common-roots/origins.yml`).

## Top-level shape

```yaml
metadata:
  title: "Session N"

origins:
  - origin: ...
    language: ...
    meaning: ...
    type: prefix | suffix          # optional
    forms: [...]                   # optional, see "Forms"

concepts:                          # optional, see "Concepts"
  - key: ...
    meaning: ...
    note: ...                      # optional
    members: [...]

relations:                         # optional, see "Relations"
  - { type: ..., between: [A, B] } # undirected
  - { type: ..., from: A, to: B }  # directed
```

## Forms

A list of inflectional / morphological variants of an origin (Latin
principal parts, French gender, Greek noun stems, German strong-verb
forms, …). Each form has at least a `form` string and a `role` string.
Role values are free strings — conventions are per-language and only need
to be consistent within a single origin's forms.

```yaml
- origin: dict
  language: Latin
  meaning: to say, to speak
  forms:
    - { form: dico,   role: present_active_indicative }
    - { form: dicere, role: present_active_infinitive }
    - { form: dictum, role: supine, note: "produces diction, dictionary" }
```

A definition that derives from one specific form can pin its origin
reference via `from_form`:

```yaml
# in a definitions notebook
- expression: dictionary
  origin_parts:
    - origin: dict
      language: Latin
      from_form: dictum   # must match one of forms[].form on the referenced origin
```

`from_form` is optional. When set, the validator checks that it matches a
form declared on the referenced origin within the same session.

## Concepts

A concept is a named grouping of origins. Use it for synonym clusters
(members share a meaning across languages) or for thematic groupings
(members that belong together but have distinct meanings, e.g., kinship
roles). The semantic relation between two concepts (antonym, hypernym, …)
lives in the `relations:` block; concepts themselves carry no
inter-concept fields.

```yaml
concepts:
  - key: goodness                  # unique per book; see "Cross-session"
    meaning: good                  # human-readable gloss
    note: positive evaluative root # optional, free text
    members:
      - { origin: bene, language: Latin }
```

### Cross-session unification

`concept.key` is unique **per book**, not per session. If the same key
appears in multiple sessions of the same book, ingestion merges them —
the resulting book-level concept has the union of members across
sessions. The `meaning` and `note` fields must match exactly across all
declarations of the same key in the same book (the validator enforces).

Members of a concept must resolve to origins declared in the **same
session** as the concept block. To add a member from a different session,
re-declare the concept in that session.

## Relations

A typed edge between two concept keys in the same book. The `type` is a
free string; the conventional vocabulary (drawn from WordNet) includes
`antonym`, `synonym`, `hypernym`, `hyponym`, `holonym`, `meronym`,
`member_of`, `has_member`, `similar_to`, `causes`, `entails`,
`derivation`, `related_to`. Any new type is accepted.

```yaml
relations:
  - { type: antonym, between: [goodness, badness] }       # symmetric
  - { type: hyponym, from: writing, to: communication-acts } # directed
```

- `between: [A, B]` for symmetric types (antonym, synonym, similar_to).
  Ingestion materialises both directions as separate DB rows so reads
  don't need `UNION`.
- `from: A, to: B` for asymmetric types (hypernym, meronym, causes, …).
- Exactly one of the two forms must be present.
- Both endpoints must be concept keys defined somewhere in the same book.

## Validation summary

`langner validate` checks the above as warnings (not errors) so existing
notebooks that don't carry these fields continue to validate cleanly:

- Forms: `form` and `role` non-empty; no duplicate `(role, form)` on the
  same origin.
- Concepts: `key` and `meaning` non-empty; cross-session declarations of
  the same key agree on `meaning` and `note`; each member resolves to an
  origin in the same session.
- Relations: exactly one of `between` / `from`+`to`; endpoints resolve.
- `from_form`: matches a form declared on the referenced origin in the
  same session.
