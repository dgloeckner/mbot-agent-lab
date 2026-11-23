# LED effects on the CyberPi's five LEDs (led_effect). The server turns names,
# colors and speeds into numbers; the robot computes each frame from the time
# since the effect started, in the main loop, so an effect runs on while the
# robot drives and never blocks.
#
# An effect is the command's args: fx (name), c and c2 ([r, g, b]), v (0..100),
# p (period in ms), ms (duration, 0 = until replaced). A frame is a list of
# five (r, g, b), LED 1 to 5.

FX_OFF = (0, 0, 0)
FX_LEFT = (0, 1)        # LEDs of blink_left; blink_right uses the other end. VERIFY side
FX_RIGHT = (3, 4)


def fx_scale(c, k):
    """Color c at brightness k (0..1). Squared: a fade then looks even to the eye."""
    k = k * k
    return (int(c[0] * k), int(c[1] * k), int(c[2] * k))


def fx_mix(a, b, k):
    return (int(a[0] + (b[0] - a[0]) * k), int(a[1] + (b[1] - a[1]) * k), int(a[2] + (b[2] - a[2]) * k))


def fx_tri(t, p):
    """0 up to 1 and back to 0 over the period p."""
    x = (t % p) / p
    return 2 * x if x < 0.5 else 2 - 2 * x


def fx_hue(h):
    """Fully saturated color of hue h (0..1)."""
    h = (h % 1) * 6
    f = h - int(h)
    up, down = int(255 * f), int(255 * (1 - f))
    return ((255, up, 0), (down, 255, 0), (0, 255, up), (0, down, 255), (up, 0, 255), (255, 0, down))[int(h) % 6]


def fx_frame(fx, t, start):
    """The five colors of effect fx at t ms after its start; start is the frame before it."""
    name = fx.get("fx")
    c = tuple(fx.get("c") or (255, 255, 255))
    p = max(int(fx.get("p") or 1000), 50)
    if name == "breathe":
        return [fx_scale(c, 0.2 + 0.8 * fx_tri(t, p))] * 5
    if name == "fade":
        k = t / p if t < p else 1
        return [fx_mix(start[i], c, k) for i in range(5)]
    if name == "chase":
        pos = (t % p) * 5 / p
        out = []
        for i in range(5):
            d = (pos - i) % 5  # how far the bright point has moved past this LED
            out.append(fx_scale(c, 1 - d / 2) if d < 2 else FX_OFF)
        return out
    if name in ("blink_left", "blink_right"):
        lit = FX_LEFT if name == "blink_left" else FX_RIGHT
        on = (t // (p // 2)) % 2 == 0
        return [c if on and i in lit else FX_OFF for i in range(5)]
    if name == "rainbow":
        return [fx_hue(t / p + i / 5) for i in range(5)]
    if name == "gradient":
        c2 = tuple(fx.get("c2") or (0, 0, 0))
        return [fx_mix(c, c2, i / 4) for i in range(5)]
    if name == "level":
        n = fx.get("v", 0) * 5 / 100
        return [fx_scale(c, min(1, max(0, n - i))) for i in range(5)]
    if name == "heartbeat":
        x = t % p
        if x < 200:
            k = 1 - x / 200
        elif 250 <= x < 450:
            k = 0.8 * (1 - (x - 250) / 200)
        else:
            k = 0
        return [fx_scale(c, max(k, 0.15))] * 5
    return [FX_OFF] * 5  # "off" and anything unknown
