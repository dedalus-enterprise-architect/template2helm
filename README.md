# Template2Helm

Template2Helm is a go utility that converts OpenShift templates into Helm charts.

*ATTENTION:* this is a forked project customized by the EA Team @Dedalus

## Maintainers

- Claudio Prato (<claudio.prato@dedalus.eu>)

## Installation

Installing is very simple. Simply download the proper binary from our latest [release](https://github.com/dedalus-enterprise-architect/template2helm/releases), and put it on your `$PATH`.

### Features

- the APIVersion is set to __v2__

- both the Chart *version* and the *appVersion* are set to the first match among the following directives:

  - the key: "__appversion__" into the template *annotations*

  - the variable: "__APP_VERSION__" into the template *Parameters*

  - the fixed value: "v0.0.1"

- set the annotation: "reloader.stakater.com/auto" on Deployment object

- keep the original *route* object

- create a generic definition of an *ingress* starting from the original *route*

- check the annotation: *reloader.stakater.com/auto*

## Usage

template2helm has one primary function, `convert`. It can be used like so to convert an OpenShift template to a Helm chart.

```bash
template2helm convert --template ./examples/slack-notify-job-template.yml --chart ~/tmp/charts
```

We have several [example templates](./examples/) you can use to get started.

## Contribution

Please open issues and pull requests! Check out our [development guide](./docs/dev_guide.md) for more info on how to get started. We also follow the general [contribution guidelines](https://redhat-cop.github.io/contrib/) for pull requests outlined on the [Red Hat Community of Practice](https://redhat-cop.github.io) website.
