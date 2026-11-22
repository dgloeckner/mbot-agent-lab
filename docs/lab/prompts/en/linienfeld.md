# Example prompt: line field, level 2

Scenario: line field (`docs/lab/feld/`), one robot. Goal: drive to junction (2,3).
The prompt is deliberately incomplete. Teams discover what is missing while testing.

---

# Field knowledge

The floor is a grid of black lines with 4 x 4 junctions, 30 cm apart.
Color stripes are digits: red 0, yellow 1, green 2, blue 3.
After a junction come two stripes: first x, then y of the junction you are leaving.
Before a junction come two stripes: first y, then x of the junction you are reaching.
(0,0) is bottom left, x grows to the east, y to the north.

# How to work

You do not know your position. No tool tells you.
Read the `floor` field after every move. The outer letters show the stripe color.
After every junction, note your position and heading with `remember`.
Keep your heading as a compass angle: north 0, east 90, south 180, west 270.
A turn to the right adds, a turn to the left subtracts.
If a color does not match what you expected, work out your position again.

# Task

Drive to junction (2,3) and report there with `finish`.
