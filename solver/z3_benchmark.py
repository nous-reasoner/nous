"""
Z3 Benchmark for NOUS CSP Problems

Replicates the deterministic CSP generation from Go (csp/generate.go) in Python,
then solves with Z3 to measure timing. Compares against Claude Opus's ~13.5s average.

Usage:
    python z3_benchmark.py
"""

import hashlib
import hmac
import math
import struct
import time
from enum import IntEnum

# ============================================================
# Crypto derivation helpers (matching crypto/derive.go)
# ============================================================

def derive_int(seed: bytes, label: str) -> int:
    """HMAC-SHA256(seed, label), return first 8 bytes as uint64."""
    mac = hmac.new(seed, label.encode('utf-8'), hashlib.sha256).digest()
    return struct.unpack('>Q', mac[:8])[0]

def derive_subset(seed: bytes, label: str, n: int, count: int) -> list:
    """Fisher-Yates shuffle to select `count` distinct indices from [0, n)."""
    if count > n:
        count = n
    pool = list(range(n))
    selected = []
    for i in range(count):
        remaining = len(pool) - i
        idx = derive_int(seed, f"{label}{i}") % remaining
        pos = int(idx) + i
        pool[i], pool[pos] = pool[pos], pool[i]
        selected.append(pool[i])
    return selected

# ============================================================
# Math helpers (matching csp/math_helpers.go)
# ============================================================

def is_prime(n: int) -> bool:
    if n < 2:
        return False
    if n < 4:
        return True
    if n % 2 == 0 or n % 3 == 0:
        return False
    i = 5
    while i * i <= n:
        if n % i == 0 or n % (i + 2) == 0:
            return False
        i += 6
    return True

def nth_prime(n: int) -> int:
    if n <= 0:
        return 2
    count = 0
    candidate = 2
    while True:
        if is_prime(candidate):
            if count == n:
                return candidate
            count += 1
        candidate += 1

def gcd(a: int, b: int) -> int:
    a, b = abs(a), abs(b)
    while b != 0:
        a, b = b, a % b
    return a

def fibonacci(n: int) -> int:
    if n <= 0:
        return 0
    if n == 1:
        return 1
    a, b = 0, 1
    for _ in range(2, n + 1):
        a, b = b, a + b
    return b

def digit_root(n: int) -> int:
    if n < 0:
        n = -n
    if n == 0:
        return 0
    return 1 + (n - 1) % 9

# ============================================================
# CSP types (matching csp/csp.go)
# ============================================================

class ConstraintType(IntEnum):
    Linear = 0
    MulMod = 1
    SumSquares = 2
    Compare = 3
    ModChain = 4
    Conditional = 5
    Trilinear = 6
    Divisible = 7
    PrimeNth = 8
    GCD = 9
    FibMod = 10
    NestedCond = 11
    XOR = 12
    DigitRoot = 13

NUM_CONSTRAINT_TYPES = 14

CONSTRAINT_WEIGHTS = [12, 10, 8, 10, 8, 7, 8, 7, 5, 6, 5, 5, 5, 4]

LEVEL_CONFIGS = {
    'Standard':  {'base_vars': 8,  'var_range': 5, 'constraint_ratio': 1.2},
    'Challenge': {'base_vars': 15, 'var_range': 8, 'constraint_ratio': 1.8},
}

def weighted_constraint_type(seed: bytes, tag: str) -> ConstraintType:
    val = derive_int(seed, tag) % 100
    cumulative = 0
    for i, w in enumerate(CONSTRAINT_WEIGHTS):
        cumulative += w
        if val < cumulative:
            return ConstraintType(i)
    return ConstraintType.Linear

# ============================================================
# Constraint builders (matching csp/generate.go)
# ============================================================

def build_linear(seed, p, n, cand):
    v = derive_subset(seed, p + "v", n, 2)
    a = int(derive_int(seed, p + "a") % 10) + 1
    b = int(derive_int(seed, p + "b") % 10) + 1
    if derive_int(seed, p + "sa") % 2 == 1:
        a = -a
    if derive_int(seed, p + "sb") % 2 == 1:
        b = -b
    c = a * cand[v[0]] + b * cand[v[1]]
    return {'type': ConstraintType.Linear, 'vars': v, 'params': [a, b, c]}

def build_mul_mod(seed, p, n, cand):
    v = derive_subset(seed, p + "v", n, 2)
    z = int(derive_int(seed, p + "z") % 50) + 2
    product = cand[v[0]] * cand[v[1]]
    w = product % z
    if w < 0:
        w += z
    return {'type': ConstraintType.MulMod, 'vars': v, 'params': [z, w]}

def build_sum_squares(seed, p, n, cand):
    v = derive_subset(seed, p + "v", n, 2)
    x, y = cand[v[0]], cand[v[1]]
    z_val = x * x + y * y
    k = int(derive_int(seed, p + "k") % 5)
    return {'type': ConstraintType.SumSquares, 'vars': v, 'params': [z_val, k]}

def build_compare(seed, p, n, cand):
    v = derive_subset(seed, p + "v", n, 2)
    k = cand[v[0]] - cand[v[1]] - 1
    return {'type': ConstraintType.Compare, 'vars': v, 'params': [k]}

def build_mod_chain(seed, p, n, cand):
    v = derive_subset(seed, p + "v", n, 2)
    x, y = cand[v[0]], cand[v[1]]
    if x == y:
        a = int(derive_int(seed, p + "a") % 20) + 2
        return {'type': ConstraintType.ModChain, 'vars': v, 'params': [a, a]}

    a = int(derive_int(seed, p + "a") % 20) + 2
    target = x % a

    for i in range(10):
        b = int(derive_int(seed, f"{p}b{i}") % 30) + 1
        if y % b == target:
            return {'type': ConstraintType.ModChain, 'vars': v, 'params': [a, b]}

    diff = abs(x - y)
    if diff < 1:
        diff = 1
    return {'type': ConstraintType.ModChain, 'vars': v, 'params': [diff, diff]}

def build_conditional(seed, p, n, cand):
    v = derive_subset(seed, p + "v", n, 2)
    x, y = cand[v[0]], cand[v[1]]
    k = int(derive_int(seed, p + "k") % 100)

    if x > k:
        m = y + 1 + int(derive_int(seed, p + "m") % 20)
        nv = int(derive_int(seed, p + "n") % 50)
    else:
        nv = y - 1 - int(derive_int(seed, p + "nd") % 20)
        m = int(derive_int(seed, p + "m") % 200) + 1
    return {'type': ConstraintType.Conditional, 'vars': v, 'params': [k, m, nv]}

def build_trilinear(seed, p, n, cand):
    if n < 3:
        return build_linear(seed, p, n, cand)
    v = derive_subset(seed, p + "v", n, 3)
    w = cand[v[0]] * cand[v[1]] + cand[v[2]]
    return {'type': ConstraintType.Trilinear, 'vars': v, 'params': [w]}

def build_divisible(seed, p, n, cand):
    for attempt in range(10):
        v = derive_subset(seed, f"{p}v{attempt}", n, 2)
        x, y = cand[v[0]], cand[v[1]]
        if y != 0 and x % y == 0:
            return {'type': ConstraintType.Divisible, 'vars': v, 'params': []}
        if x != 0 and y % x == 0:
            return {'type': ConstraintType.Divisible, 'vars': [v[1], v[0]], 'params': []}
    return build_compare(seed, p, n, cand)

def build_prime_nth(seed, p, n, cand):
    v = derive_subset(seed, p + "v", n, 2)
    N = int(derive_int(seed, p + "n") % 8) + 2
    idx = cand[v[0]] % N
    if idx < 0:
        idx += N
    k = nth_prime(idx) + cand[v[1]]
    return {'type': ConstraintType.PrimeNth, 'vars': v, 'params': [N, k]}

def build_gcd(seed, p, n, cand):
    v = derive_subset(seed, p + "v", n, 2)
    k = gcd(cand[v[0]], cand[v[1]])
    return {'type': ConstraintType.GCD, 'vars': v, 'params': [k]}

def build_fib_mod(seed, p, n, cand):
    v = derive_subset(seed, p + "v", n, 2)
    N = int(derive_int(seed, p + "n") % 15) + 2
    M = int(derive_int(seed, p + "m") % 20) + 2
    idx = cand[v[0]] % N
    if idx < 0:
        idx += N
    fib_val = fibonacci(idx)
    k = (fib_val + cand[v[1]]) % M
    if k < 0:
        k += M
    return {'type': ConstraintType.FibMod, 'vars': v, 'params': [N, M, k]}

def build_nested_cond(seed, p, n, cand):
    if n < 3:
        return build_linear(seed, p, n, cand)
    v = derive_subset(seed, p + "v", n, 3)
    x, y, z = cand[v[0]], cand[v[1]], cand[v[2]]
    a = int(derive_int(seed, p + "a") % 100)
    b = int(derive_int(seed, p + "b") % 100)

    if x > a and y < b:
        c = z - 1 - int(derive_int(seed, p + "c") % 20)
    else:
        c = int(derive_int(seed, p + "c") % 100)
    return {'type': ConstraintType.NestedCond, 'vars': v, 'params': [a, b, c]}

def build_xor(seed, p, n, cand):
    v = derive_subset(seed, p + "v", n, 2)
    k = cand[v[0]] ^ cand[v[1]]
    return {'type': ConstraintType.XOR, 'vars': v, 'params': [k]}

def build_digit_root(seed, p, n, cand):
    v = derive_subset(seed, p + "v", n, 2)
    product = cand[v[0]] * cand[v[1]]
    k = digit_root(product)
    return {'type': ConstraintType.DigitRoot, 'vars': v, 'params': [k]}

BUILDERS = {
    ConstraintType.Linear: build_linear,
    ConstraintType.MulMod: build_mul_mod,
    ConstraintType.SumSquares: build_sum_squares,
    ConstraintType.Compare: build_compare,
    ConstraintType.ModChain: build_mod_chain,
    ConstraintType.Conditional: build_conditional,
    ConstraintType.Trilinear: build_trilinear,
    ConstraintType.Divisible: build_divisible,
    ConstraintType.PrimeNth: build_prime_nth,
    ConstraintType.GCD: build_gcd,
    ConstraintType.FibMod: build_fib_mod,
    ConstraintType.NestedCond: build_nested_cond,
    ConstraintType.XOR: build_xor,
    ConstraintType.DigitRoot: build_digit_root,
}

# ============================================================
# Problem generation (matching csp/generate.go)
# ============================================================

def generate_problem(seed: bytes, level: str):
    """Generate a CSP problem deterministically from seed, matching Go implementation."""
    cfg = LEVEL_CONFIGS[level]

    # Number of variables
    num_vars = cfg['base_vars'] + int(derive_int(seed, "num_var") % cfg['var_range'])

    # Generate variables
    variables = []
    for i in range(num_vars):
        lo = int(derive_int(seed, f"lo{i}") % 50)
        hi = lo + 20 + int(derive_int(seed, f"hi{i}") % 80)
        variables.append({'name': f'x{i}', 'lower': lo, 'upper': hi})

    # Derive candidate solution
    cand = []
    for i in range(num_vars):
        dom_size = variables[i]['upper'] - variables[i]['lower'] + 1
        cand.append(variables[i]['lower'] + int(derive_int(seed, f"cand{i}") % dom_size))

    # Generate constraints
    num_constraints = int(math.ceil(num_vars * cfg['constraint_ratio']))
    constraints = []
    for c in range(num_constraints):
        ctype = weighted_constraint_type(seed, f"ctype{c}")
        p = f"c{c}_"
        builder = BUILDERS.get(ctype, build_linear)
        constraints.append(builder(seed, p, num_vars, cand))

    return {
        'variables': variables,
        'constraints': constraints,
        'level': level,
        'num_vars': num_vars,
        'num_constraints': num_constraints,
    }, cand

# ============================================================
# Z3 solver (model all 14 constraint types)
# ============================================================

def solve_with_z3(problem):
    """Solve a CSP problem using Z3. Returns (solution, elapsed_seconds)."""
    import z3

    variables = problem['variables']
    constraints = problem['constraints']
    num_vars = len(variables)

    # Create Z3 integer variables
    z3_vars = []
    solver = z3.Solver()
    solver.set("timeout", 120000)  # 120 second timeout per problem

    for i, var in enumerate(variables):
        v = z3.Int(var['name'])
        z3_vars.append(v)
        # Domain bounds
        solver.add(v >= var['lower'])
        solver.add(v <= var['upper'])

    # Add constraints
    for con in constraints:
        ct = con['type']
        vi = con['vars']
        params = con['params']

        if ct == ConstraintType.Linear:
            # a*X + b*Y = c
            a, b, c = params
            solver.add(a * z3_vars[vi[0]] + b * z3_vars[vi[1]] == c)

        elif ct == ConstraintType.MulMod:
            # X*Y mod Z = W
            z_val, w = params
            product = z3_vars[vi[0]] * z3_vars[vi[1]]
            # Z3 mod can be negative for negative operands; use python-style mod
            solver.add(product % z_val == w)

        elif ct == ConstraintType.SumSquares:
            # |X^2 + Y^2 - Z| <= k
            z_val, k = params
            x, y = z3_vars[vi[0]], z3_vars[vi[1]]
            diff = x * x + y * y - z_val
            solver.add(z3.And(diff >= -k, diff <= k))

        elif ct == ConstraintType.Compare:
            # X > Y + k
            k_val = params[0]
            solver.add(z3_vars[vi[0]] > z3_vars[vi[1]] + k_val)

        elif ct == ConstraintType.ModChain:
            # X mod A = Y mod B
            a, b = params
            solver.add(z3_vars[vi[0]] % a == z3_vars[vi[1]] % b)

        elif ct == ConstraintType.Conditional:
            # if X > k then Y < m else Y > n
            k_val, m, n = params
            x, y = z3_vars[vi[0]], z3_vars[vi[1]]
            solver.add(z3.If(x > k_val, y < m, y > n))

        elif ct == ConstraintType.Trilinear:
            # X*Y + Z = W
            w = params[0]
            solver.add(z3_vars[vi[0]] * z3_vars[vi[1]] + z3_vars[vi[2]] == w)

        elif ct == ConstraintType.Divisible:
            # X mod Y = 0, Y != 0
            x, y = z3_vars[vi[0]], z3_vars[vi[1]]
            solver.add(y != 0)
            solver.add(x % y == 0)

        elif ct == ConstraintType.PrimeNth:
            # NthPrime(X mod N) + Y = k
            # Z3 cannot compute NthPrime symbolically. We enumerate:
            # For each possible value of X mod N (0..N-1), compute NthPrime,
            # then require Y = k - NthPrime(idx).
            N, k_val = params
            x, y = z3_vars[vi[0]], z3_vars[vi[1]]
            cases = []
            for idx in range(N):
                p_val = nth_prime(idx)
                cases.append(z3.And(x % N == idx, y == k_val - p_val))
            solver.add(z3.Or(cases))

        elif ct == ConstraintType.GCD:
            # GCD(X, Y) = k
            # k must divide both X and Y. For each var, enumerate valid multiples.
            k_val = params[0]
            x, y = z3_vars[vi[0]], z3_vars[vi[1]]
            if k_val == 0:
                solver.add(x == 0)
                solver.add(y == 0)
            else:
                # k divides X and k divides Y
                solver.add(x % k_val == 0)
                solver.add(y % k_val == 0)
                # GCD(X/k, Y/k) = 1: enumerate coprime X-values for each valid X
                x_lo, x_hi = variables[vi[0]]['lower'], variables[vi[0]]['upper']
                y_lo, y_hi = variables[vi[1]]['lower'], variables[vi[1]]['upper']
                valid_x = [xv for xv in range(x_lo, x_hi + 1) if xv % k_val == 0]
                valid_y = [yv for yv in range(y_lo, y_hi + 1) if yv % k_val == 0]
                # Filter to pairs where GCD is exactly k (not a multiple of k)
                x_options = []
                for xv in valid_x:
                    y_ok = [yv for yv in valid_y if gcd(xv, yv) == k_val]
                    if y_ok:
                        x_options.append(z3.And(x == xv, z3.Or([y == yv for yv in y_ok])))
                if x_options:
                    solver.add(z3.Or(x_options))
                else:
                    solver.add(False)

        elif ct == ConstraintType.FibMod:
            # (Fibonacci(X mod N) + Y) mod M = k
            N, M, k_val = params
            x, y = z3_vars[vi[0]], z3_vars[vi[1]]
            cases = []
            for idx in range(N):
                fib_val = fibonacci(idx)
                # (fib_val + Y) mod M = k  =>  Y mod M = (k - fib_val) mod M
                target = (k_val - fib_val) % M
                cases.append(z3.And(x % N == idx, (fib_val + y) % M == k_val))
            solver.add(z3.Or(cases))

        elif ct == ConstraintType.NestedCond:
            # if X > a AND Y < b then Z > c
            a, b, c = params
            x, y, z = z3_vars[vi[0]], z3_vars[vi[1]], z3_vars[vi[2]]
            solver.add(z3.If(z3.And(x > a, y < b), z > c, True))

        elif ct == ConstraintType.XOR:
            # X XOR Y = k
            # For bounded integer domains, enumerate valid X values and compute Y.
            k_val = params[0]
            x, y = z3_vars[vi[0]], z3_vars[vi[1]]
            x_lo, x_hi = variables[vi[0]]['lower'], variables[vi[0]]['upper']
            y_lo, y_hi = variables[vi[1]]['lower'], variables[vi[1]]['upper']
            options = []
            for xv in range(x_lo, x_hi + 1):
                yv = xv ^ k_val
                if y_lo <= yv <= y_hi:
                    options.append(z3.And(x == xv, y == yv))
            if options:
                solver.add(z3.Or(options))
            else:
                solver.add(False)

        elif ct == ConstraintType.DigitRoot:
            # DigitRoot(X * Y) = k
            # DigitRoot(n) = 1 + (n-1)%9 for n>0, 0 for n=0
            # Equivalent: n%9 == k%9 (for n>0, k>0) or n==0 (k==0)
            k_val = params[0]
            x, y = z3_vars[vi[0]], z3_vars[vi[1]]
            x_lo, x_hi = variables[vi[0]]['lower'], variables[vi[0]]['upper']
            y_lo, y_hi = variables[vi[1]]['lower'], variables[vi[1]]['upper']
            # Enumerate valid X values, for each find valid Y values
            x_options = []
            for xv in range(x_lo, x_hi + 1):
                valid_ys = [yv for yv in range(y_lo, y_hi + 1)
                           if digit_root(xv * yv) == k_val]
                if valid_ys:
                    x_options.append(z3.And(x == xv, z3.Or([y == yv for yv in valid_ys])))
            if x_options:
                solver.add(z3.Or(x_options))
            else:
                solver.add(False)

    # Solve
    start = time.perf_counter()
    result = solver.check()
    elapsed = time.perf_counter() - start

    if result == z3.sat:
        model = solver.model()
        solution = [model[z3_vars[i]].as_long() for i in range(num_vars)]
        return solution, elapsed, "sat"
    elif result == z3.unknown:
        return None, elapsed, "timeout"
    else:
        return None, elapsed, "unsat"

# ============================================================
# Verification (matching csp/csp.go VerifySolution)
# ============================================================

def verify_solution(problem, solution):
    """Verify that solution satisfies all constraints (pure Python, no Z3)."""
    variables = problem['variables']
    if len(solution) != len(variables):
        return False

    # Domain check
    for i, v in enumerate(solution):
        if v < variables[i]['lower'] or v > variables[i]['upper']:
            return False, f"domain violation: x{i}={v} not in [{variables[i]['lower']}, {variables[i]['upper']}]"

    # Constraint check
    for ci, con in enumerate(problem['constraints']):
        ct = con['type']
        vi = con['vars']
        params = con['params']
        vals = solution

        ok = True
        if ct == ConstraintType.Linear:
            a, b, c = params
            ok = a * vals[vi[0]] + b * vals[vi[1]] == c
        elif ct == ConstraintType.MulMod:
            z_val, w = params
            r = (vals[vi[0]] * vals[vi[1]]) % z_val
            if r < 0: r += z_val
            ok = r == w
        elif ct == ConstraintType.SumSquares:
            z_val, k = params
            diff = vals[vi[0]]**2 + vals[vi[1]]**2 - z_val
            ok = abs(diff) <= k
        elif ct == ConstraintType.Compare:
            ok = vals[vi[0]] > vals[vi[1]] + params[0]
        elif ct == ConstraintType.ModChain:
            a, b = params
            lhs = vals[vi[0]] % a
            rhs = vals[vi[1]] % b
            if lhs < 0: lhs += a
            if rhs < 0: rhs += b
            ok = lhs == rhs
        elif ct == ConstraintType.Conditional:
            k, m, n = params
            x, y = vals[vi[0]], vals[vi[1]]
            if x > k:
                ok = y < m
            else:
                ok = y > n
        elif ct == ConstraintType.Trilinear:
            ok = vals[vi[0]] * vals[vi[1]] + vals[vi[2]] == params[0]
        elif ct == ConstraintType.Divisible:
            x, y = vals[vi[0]], vals[vi[1]]
            ok = y != 0 and x % y == 0
        elif ct == ConstraintType.PrimeNth:
            N, k = params
            idx = vals[vi[0]] % N
            if idx < 0: idx += N
            ok = nth_prime(idx) + vals[vi[1]] == k
        elif ct == ConstraintType.GCD:
            ok = gcd(vals[vi[0]], vals[vi[1]]) == params[0]
        elif ct == ConstraintType.FibMod:
            N, M, k = params
            idx = vals[vi[0]] % N
            if idx < 0: idx += N
            result = (fibonacci(idx) + vals[vi[1]]) % M
            if result < 0: result += M
            ok = result == k
        elif ct == ConstraintType.NestedCond:
            a, b, c = params
            x, y, z = vals[vi[0]], vals[vi[1]], vals[vi[2]]
            if x > a and y < b:
                ok = z > c
            else:
                ok = True
        elif ct == ConstraintType.XOR:
            ok = vals[vi[0]] ^ vals[vi[1]] == params[0]
        elif ct == ConstraintType.DigitRoot:
            ok = digit_root(vals[vi[0]] * vals[vi[1]]) == params[0]

        if not ok:
            return False, f"constraint {ci} ({ConstraintType(ct).name}) failed"

    return True, "ok"

# ============================================================
# Main benchmark
# ============================================================

def make_seed(i: int) -> bytes:
    """Create a 32-byte seed from an integer (SHA-256 hash)."""
    return hashlib.sha256(f"benchmark_seed_{i}".encode()).digest()

def constraint_type_stats(problem):
    """Count constraint types in a problem."""
    counts = {}
    for con in problem['constraints']:
        name = ConstraintType(con['type']).name
        counts[name] = counts.get(name, 0) + 1
    return counts

def run_benchmark():
    import sys
    num_trials_standard = 5
    num_trials_challenge = 5

    print("=" * 70, flush=True)
    print("NOUS CSP - Z3 Solver Benchmark", flush=True)
    print("=" * 70, flush=True)
    print(flush=True)

    for level in ['Standard', 'Challenge']:
        cfg = LEVEL_CONFIGS[level]
        num_trials = num_trials_standard if level == 'Standard' else num_trials_challenge
        print(f"--- {level} Level ---", flush=True)
        print(f"  Base variables: {cfg['base_vars']}, range: +{cfg['var_range']}", flush=True)
        print(f"  Constraint ratio: {cfg['constraint_ratio']}", flush=True)
        print(flush=True)

        times = []
        all_verified = True

        for trial in range(num_trials):
            seed = make_seed(trial)

            gen_start = time.perf_counter()
            problem, candidate = generate_problem(seed, level)
            gen_time = time.perf_counter() - gen_start

            # First verify the candidate solution (sanity check)
            ok, msg = verify_solution(problem, candidate)
            if not ok:
                print(f"  [FAIL] Trial {trial}: candidate verification failed: {msg}", flush=True)
                continue

            # Solve with Z3
            solution, elapsed, z3_status = solve_with_z3(problem)

            if z3_status == "sat" and solution is not None:
                # Verify Z3's solution
                ok, msg = verify_solution(problem, solution)
                status = "PASS" if ok else f"FAIL({msg})"
                if not ok:
                    all_verified = False
            else:
                status = z3_status.upper()
                all_verified = False

            times.append(elapsed)

            # Count constraint types for first trial
            if trial == 0:
                stats = constraint_type_stats(problem)
                print(f"  Problem shape: {problem['num_vars']} vars, "
                      f"{problem['num_constraints']} constraints", flush=True)
                print(f"  Constraint types: {stats}", flush=True)
                print(f"  (generation time: {gen_time*1000:.1f} ms)", flush=True)
                print(flush=True)

            print(f"  Trial {trial:2d}: {elapsed*1000:8.2f} ms  [{status}]  "
                  f"(vars={problem['num_vars']}, cons={problem['num_constraints']})", flush=True)

        if times:
            avg = sum(times) / len(times)
            min_t = min(times)
            max_t = max(times)
            median_t = sorted(times)[len(times)//2]
            print(flush=True)
            print(f"  Summary ({level}):", flush=True)
            print(f"    Trials:  {len(times)}", flush=True)
            print(f"    Average: {avg*1000:.2f} ms", flush=True)
            print(f"    Median:  {median_t*1000:.2f} ms", flush=True)
            print(f"    Min:     {min_t*1000:.2f} ms", flush=True)
            print(f"    Max:     {max_t*1000:.2f} ms", flush=True)
            print(f"    All verified: {all_verified}", flush=True)
            print(flush=True)

    print()
    print("=" * 70)
    print("Comparison:")
    print(f"  Claude Opus (AI):  ~13,500 ms average (standard level)")
    print(f"  Z3 (SMT solver):   see results above")
    print("=" * 70)

if __name__ == '__main__':
    run_benchmark()
