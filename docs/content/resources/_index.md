---
type: docs
title: "Radius resource library"
linkTitle: "Resource library"
description: "Learn about the different resources you can use to model your application with"
weight: 30
no_list: true
---

Applications are made up of resources. Resources can be abstract and deployable to multiple Radius platforms (*eg. Container, MongoDB, etc*), or they can be platform specific and deployable to a single platform (*Azure CosmosDB, Kubernetes DaemonSet, etc*).

{{% alert title="Learn about the Radius app model" color="info" %}}
To learn more about the Radius application model an how to define resources and relationships visit the [Radius concepts]({{< ref appmodel-concept >}}).
{{% /alert %}}

## Resource types

{{< cardpane >}}
{{< card header="**Services**" >}}
Model your running code with services.<br /><br />
[Learn more]({{< ref services >}})
{{< /card >}}
{{< card header="**Networking**" >}}
Define your network relationships & requirements.<br /><br />
[Learn more]({{< ref networking >}})
{{< /card >}}
{{< card header="**Connectors**" >}}
Add portability to your application with connectors.<br /><br />
[Learn more]({{< ref connectors >}})
{{< /card >}}
{{< /cardpane >}}
{{< cardpane >}}
{{< card header="**Kubernetes**" >}}
Model and connect to Kubernetes resources.<br /><br />
[Learn more]({{< ref services >}})
{{< /card >}}
{{< card header="**Microsoft Azure**" >}}
Content card 3
{{< /card >}}
{{< card header="**Custom/3rd Party**" >}}
Content card 3
{{< /card >}}
{{< /cardpane >}}