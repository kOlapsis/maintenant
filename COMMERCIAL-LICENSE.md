# Maintenant Commercial License

**Version 3.0 -- September 2026**

This Commercial License Agreement ("Agreement") is between Benjamin Touchard,
operating as Kolapsis ("Licensor"), and the individual or entity that purchased
a Maintenant Personal license or subscribes to a Maintenant Pro plan
("Licensee").

## 1. What this Agreement covers

Maintenant is made of two parts:

- **The core**, licensed under the Apache License, Version 2.0 (see `LICENSE`).
  It is free for any use, commercial or not, and needs no key. It is the
  Community edition.
- **The commercial code**, under `internal/commercial/` and
  `frontend/src/commercial/`, licensed under the Maintenant Commercial Source
  License (see `internal/commercial/LICENSE`). It implements the features a
  Personal or Pro licence key opens.

Both parts ship in the same binary. This Agreement sets the terms under which
a Personal or Pro key may be used to run the commercial code in production.

## 2. Personal

### 2.1 Grant

The Licensor grants a **non-exclusive, non-transferable, perpetual** license to
one named individual to:

1. Use the features the Personal edition unlocks, on infrastructure that
   individual owns or operates **for their own account**. This includes a
   freelancer or sole trader monitoring their own stack.

### 2.2 What Personal does not cover

The Personal license does **not** grant the right to use the Personal
features to:

- Monitor infrastructure belonging to a third party, whether a client, an
  employer, or any other organisation;
- Provide monitoring, reporting, or operations as a service to others, paid or
  unpaid, using Maintenant;
- Serve a company or team beyond the named individual.

Any of those uses requires a Pro subscription. The commercial code may not be
redistributed, sublicensed, or resold under any edition.

There is no technical enforcement of this clause. It is a legal term and rests
on trust.

### 2.3 Perpetuity and product updates

The Personal license **never expires**. It is bought once and is not a
subscription.

It includes **one year of product updates** from the date of purchase:

- Every version released **within** that year stays licensed **for life**. The
  last version released before the window closes may be run indefinitely.
- Versions released **after** the window closes are not covered until the
  Licensee buys another year.
- A renewal extends the window by one further year, counted from the end of the
  current window when it is still open, so renewing early costs nothing.

Nothing stops working when the window closes. The installed version keeps
running under this license; only newer releases fall outside it.

### 2.4 Support

The Personal license carries **no support commitment**.

## 3. Pro

### 3.1 Grant

The Licensor grants a **non-exclusive, non-transferable, worldwide** license,
for the duration of the subscription, to:

1. Use the features the Pro edition unlocks, in production,
   **commercially**, including on behalf of an employer, a team, or third
   parties.
2. Receive email support at license@maintenant.dev.

### 3.2 Scope

- Each Pro subscription covers **one deployment instance** of Maintenant.
- Additional instances require additional subscriptions.
- The license does **not** grant the right to redistribute, sublicense, or
  resell the commercial code.

### 3.3 Term

- This license is valid **for the duration of the active subscription**, and
  product updates are included throughout it.
- If the subscription lapses, the instance falls back to the Community edition
  and the Pro features stop. The core remains usable under the Apache License,
  Version 2.0.

## 4. Pricing

| Plan                     | Price               |
|--------------------------|---------------------|
| Personal                 | 149 EUR, once       |
| Personal, extra year of updates | 59 EUR / year |
| Pro Monthly              | 29 EUR / month      |
| Pro Annual               | 290 EUR / year      |

Purchases and subscriptions are managed at
[maintenant.dev](https://maintenant.dev).

## 5. Termination

- The Licensor may terminate this Agreement immediately if the Licensee
  breaches its terms.
- A refunded Personal purchase cancels the license granted by it.

## 6. Warranty Disclaimer

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED. THE LICENSOR SHALL NOT BE LIABLE FOR ANY CLAIM, DAMAGES, OR OTHER
LIABILITY ARISING FROM THE USE OF THE SOFTWARE.

## 7. Contact

For licensing questions, volume pricing, or custom agreements:

**license@maintenant.dev**
