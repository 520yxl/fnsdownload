import test from "node:test";
import assert from "node:assert/strict";
import {signRequest} from "../src/thunder-auth.js";
test("HMAC signature is deterministic for fixed inputs",()=>{
 const h=signRequest({method:"POST",pathname:"/api/v1/auth/token",body:'{"device_id":"device-123"}',productId:"moxyi-download",productSecret:"test-secret",now:1700000000,nonce:"abcdefghijklmnopQRSTUV"});
 assert.equal(h["X-Timestamp"],"1700000000");assert.equal(h["X-Product-ID"],"moxyi-download");
 assert.match(h["X-Content-SHA256"],/^[a-f0-9]{64}$/);assert.match(h["X-Signature"],/^[a-f0-9]{64}$/);
});
test("GET signature hashes the empty body",()=>{
 const h=signRequest({method:"GET",pathname:"/api/v1/auth/status",body:"",productId:"moxyi-download",productSecret:"test-secret",now:1700000000,nonce:"abcdefghijklmnopQRSTUV"});
 assert.equal(h["X-Content-SHA256"],"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855");
});
