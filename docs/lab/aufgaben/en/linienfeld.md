---
title: Line field – drive to junction (2,3)
---

## Task

The floor is a grid of black lines with 4 × 4 junctions, 30 cm apart.
Between the junctions there are color stripes. They are a code:

- red = 0, yellow = 1, green = 2, blue = 3
- After a junction come two stripes: first x, then y of the junction you are leaving.
- Before a junction come two stripes: first y, then x of the junction you are reaching.
- (0,0) is bottom left. x grows to the east, y to the north.

Your robot starts at a junction it does not know. **Get it to junction (2,3).** There it reports
with `finish`.

No tool tells the agent where it is. It only sees which colors passed under the robot. How it
works out its position from that has to be in your prompt: your agent does not know this text.

## Starter prompt

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
