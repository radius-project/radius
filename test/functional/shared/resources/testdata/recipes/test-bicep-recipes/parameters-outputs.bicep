// This is the simplest possible Bicep recipe. It doesn't do anything useful.

#disable-next-line no-unused-params
param context object

param a string
param b string = 'default value'
param c int
param d string = 'default value'

output result object = {
  values: {
    a: a
    b: b
    c: c
    d: d
  }
  secrets: {
    e: 'so secret'
  }
}
