# Visual Style — Resource Types

The renderer themes each node by its Radius resource type with a Primer-aligned
color pair plus a Unicode icon. In single-graph mode the type palette replaces
the muted "unchanged" gray fill so each resource type is visually distinct; in
diff mode the diff colors win so the diff signal stays dominant.

The mapping lives in `TYPE_STYLES` and `styleForType()` near the top of
[`../template/app-graph.html.tmpl`](../template/app-graph.html.tmpl). Update
both this table and the template together when adding a new resource type.

## Type table

| Resource type                       | Icon | Fill      | Border    | Legend label         |
| ----------------------------------- | ---- | --------- | --------- | -------------------- |
| `Applications.Core/applications`    | 📦   | `#ddf4ff` | `#0969da` | Application          |
| `Radius.Compute/containers`         | 🐳   | `#dbeefb` | `#1f6feb` | Container            |
| `Radius.Compute/containerImages`    | 🖼️   | `#fbefff` | `#8250df` | Container image      |
| `Radius.Compute/gateways`           | 🌐   | `#e6f4ff` | `#218bff` | Gateway              |
| `Radius.Data/mySqlDatabases`        | 🐬   | `#fff1e5` | `#bc4c00` | MySQL database       |
| `Radius.Data/postgreSqlDatabases`   | 🐘   | `#eaf4ff` | `#0550ae` | PostgreSQL database  |
| `Radius.Data/mongoDatabases`        | 🍃   | `#dafbe1` | `#1a7f37` | Mongo database       |
| `Radius.Data/redisCaches`           | 🧱   | `#ffebe9` | `#cf222e` | Redis cache          |
| `Radius.Security/secrets`           | 🔐   | `#fff8c5` | `#9a6700` | Secret               |

## Provider-prefix fallbacks

When a resource type is not in the table above, `styleForType()` falls back to
a category style based on the provider prefix:

| Provider prefix       | Icon | Fill      | Border    | Legend label       |
| --------------------- | ---- | --------- | --------- | ------------------ |
| `Radius.Compute/*`    | ⚙️    | `#dbeefb` | `#1f6feb` | Compute resource   |
| `Radius.Data/*`       | 🗄️   | `#fff1e5` | `#bc4c00` | Data resource      |
| `Radius.Security/*`   | 🔐   | `#fff8c5` | `#9a6700` | Security resource  |
| `aws.*`               | ☁️   | `#fff5cc` | `#d29922` | AWS resource       |
| _everything else_     | 📄   | `#f6f8fa` | `#57606a` | Resource           |

## Label format

```
<icon>  <resource.name>
<shortType>
```

The icon is prepended to the first label line. `shortType` is
`resource.type.split('/').pop()`, unchanged from the verbatim ported renderer.

## Popup format

The detail popup uses the same icon prefix on its title:

```
<icon>  <resource.name>
<resource.type>
```

## Legend

The legend at the top of the viewer is built dynamically from the resource
types actually present in the rendered graph (deduplicated, sorted). It shows
a colored swatch containing the icon, followed by the human-readable label
from the tables above. A swatch's `title` attribute carries the full resource
type for hover discoverability.
