var express = require('express');
var router = express.Router();
const axios = require('axios').default;
const inventoryService = process.env.INVENTORY_SERVICE_NAME || 'go-app';
const daprPort = process.env.DAPR_HTTP_PORT || 3500;

//use dapr http proxy (header) to call inventory service with normal /inventory route URL in axios.get call
const daprSidecar = `http://localhost:${daprPort}`
//const daprSidecar = `http://localhost:${daprPort}/v1.0/invoke/${inventoryService}/method`

/* GET users listing. */
router.get('/', async function(req, res, next) {

  var data = await axios.get(`${daprSidecar}/inventory?id=${req.query.id}`, {
    headers: {'dapr-app-id' : `${inventoryService}`} //sets app name for service discovery
  });

  res.send(`Inventory status for ${req.query.id}:\n${data.data}`);
});

module.exports = router;
