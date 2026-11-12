# Small-size mark: options for the owner

Proposal, 2026-11-12. Nothing here is in use. The favicon and every other mark file are unchanged.

## Why

`site/favicon.ico` is rendered from the Studio mark at 16, 32 and 48 px. At 16 px the equator (a 14-unit solid arc and a 6-unit dashed arc on a 512 grid) is thinner than one pixel and becomes a grey smear across the sphere. At 24 px it is two faint lines. At 32 px it reads. The collars and the red flange hold at every size. The header mark on the site is 44 px and is not affected.

## Options

`options.png` shows each option at 16, 24 and 32 px, actual size and enlarged without smoothing.

| Option | Change | At 16 px |
| --- | --- | --- |
| A, `a-current.svg` | None. The Studio mark as it is. | Equator smears. |
| B, `b-solid-equator.svg` | Front equator arc thickened to the sphere's stroke width; dashed back arc removed. | One clear line; still reads as a sphere. |
| C, `c-no-equator.svg` | Equator removed. | Cleanest, but an empty ring with four ports is close to the gun-sight reading the guide warns about. |

Recommendation: B, used only below 32 px (the 16 px favicon frame and any 24 px use). The full mark stays everywhere else.

## If one is chosen

The mark belongs to Studio. Make the small variant there (subcult-tv `studio/brands/prichal/`), rebuild the pack, then copy the result here and update `brand/studio-assets.json` and the "Below 24 px" line in `brand/GUIDE.md`. Delete this folder afterwards. The SVGs here are sketches on the mark's 512 grid, not masters.
