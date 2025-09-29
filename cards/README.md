# Make a new card
Format:
```json
{
    "id": "card_000061",
    "default": false,
    "front": {
        "type": "rich_text",
        "content": "Equation for the Mean of Grouped Data"
    },
    "back": {
        "type": "rich_text",
        "content": "Mean = $\\displaystyle \\frac{Σf x}{Σf}$,\n\nwhere f = frequency and x = midpoint of class"
    },
    "tags": ["Y1", "statistics", "data presentation and interpretation"]
}
```

__Note__:
- Each card should have tags for:
    - The year: "y1" or "y2"
    - The exam: "statistics", "mechanics" or "pure"
    - The topic title (from the cheat sheet): e.g. "data presentation and interpretation"

(with the exception of cards with a single "large data set" tag)

# Assign cards to students

Customise the assign_cards.sql

# Add image assets to cards

Use the `asset://` scheme inside rich_text to render images from a card’s declared assets. The backend parses a compact pipe syntax to set size and alignment.

## 1) Declare the asset on the card

Add the image to the card’s `assets` array (id must match the file name in the images bucket or local images folder):


## 2) Reference it from content

Basic form:
- `asset://<id>`  
  Renders the image at its natural size (scaled to fit container).

Optional modifiers (pipe-separated: "|"):
- Size (segment 2)
  - Width percent: `50%`
  - Width px: `300`
  - Width x height px: `300x180`
- Alignment (segment 3)
  - `left`, `center`, or `right`

You can combine them. If you want alignment without a size, leave the size segment empty.

Examples:
- `asset://large-data-set-map-uk.jpg` (default)
- `asset://large-data-set-map-uk.jpg|50%` (half width)
- `asset://large-data-set-map-uk.jpg|300` (300 px wide)
- `asset://large-data-set-map-uk.jpg|300x180` (fixed size)
- `asset://large-data-set-map-uk.jpg||center` (centered, default size)
- `asset://large-data-set-map-uk.jpg|50%|center` (half width and centered)

Tip: Put each `asset://...` on its own line in the rich text for predictable layout.

## 3) End-to-end example

```json
{
  "id": "card_000027",
  "default": true,
  "front": { "type": "rich_text", "content": "Cambourne:\n\nGeographical location?" },
  "back": {
    "type": "rich_text",
    "content": "Coastal UK, Northern Hemisphere\n\nasset://large-data-set-map-uk.jpg|50%|center"
  },
  "assets": [
    { "id": "large-data-set-map-uk.jpg", "type": "image", "alt": "Large data set map (UK)" }
  ],
  "tags": ["large data set"]
}
```

## Troubleshooting

- Image doesn’t render:
  - Ensure the `id` in content exactly matches an entry in the card’s `assets` array.
  - Remove stray spaces: use `asset://...`, not ` sset://...` or with a leading space.
  - Verify the file exists in the configured bucket/folder.
- Sizing/alignment ignored:
  - Check the pipe syntax order: `asset://id|<size>|<align>`.
  - Percent values must include `%` (e.g.,