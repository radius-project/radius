# Configuring Azure Open AI 

## Set up
1. Log into your azure portal and create a Azure OpenAI resource https://learn.microsoft.com/en-us/azure/ai-services/openai/how-to/create-resource?pivots=web-portal

2. Deploy a desired model https://learn.microsoft.com/en-us/azure/ai-services/openai/how-to/create-resource?pivots=web-portal#deploy-a-model 

3. Note down your API key and Endpoint by navigating to <your_az_open_ai_resource> --> Resource Management --> Keys and Endpoints

<!--
    Note: some of this content is synchronized with the prerequisites guide for simplicity. Keep these in sync!
-->

## Required Code setup 

1. Create a .env.local file in the home directory of your typescript project and add below entries

```
OPENAI_ENDPOINT=https://<your-az-openai-resource-name>.openai.azure.com/
OPENAI_API_KEY=<your api key>                                                                                                        
```
   
2. 

// The name of your Azure OpenAI Resource.
// https://learn.microsoft.com/en-us/azure/cognitive-services/openai/how-to/create-resource?pivots=web-portal#create-a-resource
const resource = 'process.env['OPENAI_API_KEY'];

// Corresponds to your Model deployment within your OpenAI resource, e.g. my-gpt35-16k-deployment
// Navigate to the Azure OpenAI Studio to deploy a model.
const model = 'nit-rad-az-openai';

// https://learn.microsoft.com/en-us/azure/ai-services/openai/reference#rest-api-versioning
const apiVersion = '2023-09-01-preview';

const apiKey = process.env['OPENAI_API_KEY'];
 
// create a OpenAI object using above parameters
const openai = new OpenAI({
    apiKey,
    baseURL: `https://${resource}.openai.azure.com/openai/deployments/${model}`,
    defaultQuery: { 'api-version': apiVersion },
    defaultHeaders: { 'api-key': apiKey },
  });


