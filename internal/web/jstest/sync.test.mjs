import {test} from "node:test";
import assert from "node:assert/strict";
import {createRequire} from "node:module";

const {queue} = createRequire(import.meta.url)("../static/sync.js");
const day = 24 * 60 * 60 * 1000;

test("only the current identity's attempts are sent; others wait 30 days, then go", () => {
  const now = 100 * day;
  const items = [
    {id: "mine", identity: "user:1", at: now - day, synced: false},
    {id: "mine-old", identity: "user:1", at: now - 40 * day, synced: false},
    {id: "anon", identity: "anon:ab", at: now - day, synced: false},
    {id: "anon-old", identity: "anon:ab", at: now - 31 * day, synced: false},
    {id: "other-user", identity: "user:2", at: now - 2 * day, synced: false},
    {id: "untagged", at: now - day, synced: false},
    {id: "untagged-old", at: now - 31 * day, synced: false},
    {id: "done", identity: "anon:ab", at: now - 40 * day, synced: true},
  ];
  const q = queue(items, "user:1", now);
  assert.deepEqual(q.send.map((a) => a.id), ["mine", "mine-old"]);
  assert.deepEqual(q.drop.map((a) => a.id), ["anon-old", "untagged-old"]);
  const back = queue(items, "anon:ab", now);
  assert.deepEqual(back.send.map((a) => a.id), ["anon", "anon-old"]);
  assert.deepEqual(queue(items, "", now).send, []);
});
