module child 'child-secure.bicep' = {
  name: 'securechild'
}

output len int = length(child.outputs.s)
