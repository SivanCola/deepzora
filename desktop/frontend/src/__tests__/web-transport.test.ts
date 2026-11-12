import assert from "node:assert/strict";
import { WebConnection } from "../lib/webTransport";
import { DESKTOP_CONTRACT_DIGEST, DESKTOP_PROTOCOL_VERSION } from "../generated/desktopContract.generated";

class FakeSocket {
  onmessage?: (event: { data: string }) => void;
  onclose?: () => void;
  onerror?: () => void;
  sent: string[] = [];
  closed = false;
  send(value: string) { this.sent.push(value); }
  close() { this.closed = true; }
  receive(value: unknown) { this.onmessage?.({ data: JSON.stringify(value) }); }
}

const socket = new FakeSocket();
const failures: Error[] = [];
const connection = new WebConnection(socket as unknown as WebSocket, error => failures.push(error));
socket.receive({ ready: true, digest: DESKTOP_CONTRACT_DIGEST, protocolVersion: DESKTOP_PROTOCOL_VERSION });
await connection.ready;
const result = connection.invoke("Version", []);
await Promise.resolve();
assert.deepEqual(JSON.parse(socket.sent[0]), { id: 1, method: "Version", args: [] });
socket.receive({ id: 1, value: "v-test" });
assert.equal(await result, "v-test");
let event: unknown;
const off = connection.on("agent:event", value => { event = value; });
socket.receive({ event: "agent:event", args: [{ kind: "text", text: "real" }] });
assert.deepEqual(event, { kind: "text", text: "real" });
off();
socket.receive({ event: "agent:event", args: ["ignored"] });
assert.deepEqual(event, { kind: "text", text: "real" });
const failed = connection.invoke("Submit", []);
await Promise.resolve();
socket.receive({ id: 2, error: { message: "lease held", code: -32000, data: { reason: "lease_held" } } });
await assert.rejects(failed, error => (error as Error & { code: number }).code === -32000 && (error as Error).message === "lease held");
const interrupted = connection.invoke("Submit", ["once"]);
await Promise.resolve();
socket.onclose?.();
await assert.rejects(interrupted, /connection closed/);
await assert.rejects(connection.invoke("Version", []), /connection closed/);
assert.equal(socket.sent.length, 3, "never replay accepted mutations after disconnect");
assert.equal(failures.length, 1);
const mismatchSocket = new FakeSocket();
const mismatch = new WebConnection(mismatchSocket as unknown as WebSocket, () => {});
mismatchSocket.receive({ ready: true, protocolVersion: DESKTOP_PROTOCOL_VERSION, digest: "different" });
await assert.rejects(mismatch.ready, /versions differ/);
assert.equal(mismatchSocket.closed, true);
console.log("PASS authenticated Web transport contract, events, structured errors and disconnect behavior");
