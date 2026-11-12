import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import test from "node:test";
import { validateCertumConfiguration } from "./validate-certum-configuration.mjs";

const configuration = {
  CERTUM_USERNAME: "developer@example.invalid",
  CERTUM_KEY_ID: "a".repeat(40),
  CERTUM_OTP_URI: "otpauth://totp/SimplySign:developer?secret=JBSWY3DPEHPK3PXP&algorithm=SHA256&digits=6&period=30",
};

test("accepts provisioning parameters used by the pinned login action", () => {
  for (const algorithm of ["SHA1", "SHA256", "SHA512", "sha256"]) {
    assert.deepEqual(validateCertumConfiguration({ ...configuration,
      CERTUM_OTP_URI: configuration.CERTUM_OTP_URI.replace("SHA256", algorithm) }), []);
  }
  assert.deepEqual(validateCertumConfiguration({ ...configuration,
    CERTUM_OTP_URI: "otpauth://totp/SimplySign?secret=JBSWY3DPEHPK3PXP" }), []);
});

test("reports all missing credentials before trying to authenticate", () => {
  const errors = validateCertumConfiguration({});
  assert.equal(errors.length, 3);
  for (const name of Object.keys(configuration)) assert.ok(errors.some(error => error.startsWith(name)));
});

test("rejects phone codes, broken URIs and ambiguous or unsupported TOTP parameters", () => {
  for (const uri of ["123456", "otpauth://totp/account", "otpauth://totp/account?secret=123456",
    "https://example.invalid/?secret=JBSWY3DPEHPK3PXP", configuration.CERTUM_OTP_URI + "&secret=ABC",
    configuration.CERTUM_OTP_URI.replace("SHA256", "MD5"), configuration.CERTUM_OTP_URI.replace("digits=6", "digits=9"),
    configuration.CERTUM_OTP_URI.replace("period=30", "period=0"), configuration.CERTUM_OTP_URI + "\n"]) {
    assert.equal(validateCertumConfiguration({ ...configuration, CERTUM_OTP_URI: uri }).length, 1);
  }
});

test("CLI fails closed and never prints credentials, parser errors or tokens", () => {
  for (const uri of [configuration.CERTUM_OTP_URI, "123456", "http://[private-provisioning-data"]) {
    const result = spawnSync(process.execPath, [new URL("./validate-certum-configuration.mjs", import.meta.url).pathname], {
      encoding: "utf8", env: { ...process.env, ...configuration, CERTUM_OTP_URI: uri },
    });
    assert.equal(result.status, uri === configuration.CERTUM_OTP_URI ? 0 : 1);
    const output = result.stdout + result.stderr;
    for (const secret of [configuration.CERTUM_USERNAME, configuration.CERTUM_KEY_ID,
      "JBSWY3DPEHPK3PXP", "123456", "private-provisioning-data"]) assert.ok(!output.includes(secret));
    assert.doesNotMatch(output, /at new URL|ERR_INVALID_URL/);
  }
});
