import { pathToFileURL } from "node:url";

// Validate without generating or printing a token. Parser exceptions can include
// the provisioning URI, so only fixed diagnostics may cross this boundary.
export function validateCertumConfiguration(env) {
  const errors = [];
  if (!env.CERTUM_USERNAME?.trim()) errors.push("CERTUM_USERNAME is missing");
  if (!/^[0-9a-fA-F]{40}$/.test(env.CERTUM_KEY_ID ?? "")) {
    errors.push("CERTUM_KEY_ID must be the 40-character SHA-1 certificate thumbprint");
  }
  const value = env.CERTUM_OTP_URI;
  if (!value) {
    errors.push("CERTUM_OTP_URI is missing; unattended signing needs the original TOTP provisioning URI, not a phone verification code");
  } else {
    let valid = false;
    try {
      const uri = new URL(value);
      const parameters = uri.searchParams;
      const secret = parameters.get("secret") ?? "";
      valid = value.startsWith("otpauth://totp/") && !/\s/.test(value)
        && uri.hostname === "totp" && !uri.username && !uri.password && !uri.port && !uri.hash
        && uri.pathname.length > 1 && /^[A-Z2-7]+={0,6}$/i.test(secret)
        && ["secret", "algorithm", "digits", "period"].every(name => parameters.getAll(name).length <= 1)
        && (!parameters.has("algorithm") || ["SHA1", "SHA256", "SHA512"].includes(parameters.get("algorithm").toUpperCase()))
        && (!parameters.has("digits") || /^[678]$/.test(parameters.get("digits")))
        && (!parameters.has("period") || /^[1-9]\d{0,3}$/.test(parameters.get("period")));
    } catch {
      // Never expose URL parsing errors or provisioning data.
    }
    if (!valid) errors.push("CERTUM_OTP_URI must be a complete otpauth://totp/ URI with a Base32 secret and supported TOTP parameters");
  }
  return errors;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const errors = validateCertumConfiguration(process.env);
  if (errors.length) {
    for (const error of errors) console.error(error);
    process.exitCode = 1;
  } else {
    console.log("Certum unattended signing configuration format is valid; live signing must still verify the account and certificate.");
  }
}
