import { Component, useMemo, useRef, type ReactNode } from 'react'
import { Canvas, useFrame } from '@react-three/fiber'
import { Float, Grid } from '@react-three/drei'
import * as THREE from 'three'
import type { BackgroundStyle } from '../theme/types'

interface BackdropProps {
  accent?: string
  light?: boolean
  intensity?: number
  speed?: number
  style?: BackgroundStyle
}

/** Scenery that is pure CSS (no WebGL) so it also works on weak devices. */
const cssOnly: BackgroundStyle[] = ['aurora', 'minimal']

let webgl: boolean | undefined

/** Whether this browser can create a WebGL context (some hardened or remote ones cannot). */
function webglAvailable(): boolean {
  if (webgl === undefined) {
    try {
      const context = document.createElement('canvas').getContext('webgl2') ?? document.createElement('canvas').getContext('webgl')
      webgl = Boolean(context)
      context?.getExtension('WEBGL_lose_context')?.loseContext()
    } catch {
      webgl = false
    }
  }
  return webgl
}

/** A scenery that fails must only lose the scenery, never unmount the application. */
class SceneryBoundary extends Component<{ fallback: ReactNode; children: ReactNode }, { failed: boolean }> {
  state = { failed: false }

  static getDerivedStateFromError() {
    return { failed: true }
  }

  render() {
    return this.state.failed ? this.props.fallback : this.props.children
  }
}

/** A slowly drifting particle field. */
function Particles({ count = 520, color, speed = 1 }: { count?: number; color: string; speed?: number }) {
  const points = useRef<THREE.Points>(null)

  const geometry = useMemo(() => {
    const positions = new Float32Array(count * 3)
    for (let i = 0; i < count; i += 1) {
      const radius = 5 + Math.random() * 12
      const theta = Math.random() * Math.PI * 2
      const phi = Math.acos(2 * Math.random() - 1)
      positions[i * 3] = radius * Math.sin(phi) * Math.cos(theta)
      positions[i * 3 + 1] = (Math.random() - 0.25) * 9
      positions[i * 3 + 2] = radius * Math.sin(phi) * Math.sin(theta)
    }
    const buffer = new THREE.BufferGeometry()
    buffer.setAttribute('position', new THREE.BufferAttribute(positions, 3))
    return buffer
  }, [count])

  useFrame((state) => {
    if (!points.current) return
    points.current.rotation.y = state.clock.elapsedTime * 0.035 * speed
    points.current.rotation.x = Math.sin(state.clock.elapsedTime * 0.05 * speed) * 0.06
  })

  return (
    <points ref={points} geometry={geometry}>
      <pointsMaterial
        size={0.055}
        color={color}
        transparent
        opacity={0.75}
        sizeAttenuation
        depthWrite={false}
        blending={THREE.AdditiveBlending}
      />
    </points>
  )
}

/** A big wireframe solid that anchors the scene. */
function Core({ color, speed = 1 }: { color: string; speed?: number }) {
  const mesh = useRef<THREE.Mesh>(null)
  useFrame((state) => {
    if (!mesh.current) return
    const t = state.clock.elapsedTime * speed
    mesh.current.rotation.y = t * 0.12
    mesh.current.rotation.x = Math.sin(t * 0.18) * 0.22
  })
  return (
    <mesh ref={mesh} position={[0, 0.2, -4.2]}>
      <icosahedronGeometry args={[2.35, 1]} />
      <meshBasicMaterial color={color} wireframe transparent opacity={0.32} />
    </mesh>
  )
}

/** Counter-rotating rings for depth. */
function Halo({ color, speed = 1 }: { color: string; speed?: number }) {
  const group = useRef<THREE.Group>(null)
  useFrame((state) => {
    if (!group.current) return
    group.current.rotation.z = state.clock.elapsedTime * 0.06 * speed
    group.current.rotation.x = Math.PI / 2.4 + Math.sin(state.clock.elapsedTime * 0.1 * speed) * 0.08
  })
  return (
    <group ref={group} position={[0, -1.4, -3]}>
      <mesh>
        <torusGeometry args={[6.4, 0.012, 8, 160]} />
        <meshBasicMaterial color={color} transparent opacity={0.5} />
      </mesh>
      <mesh rotation={[0, 0, 0.6]}>
        <torusGeometry args={[8.1, 0.008, 8, 160]} />
        <meshBasicMaterial color={color} transparent opacity={0.28} />
      </mesh>
    </group>
  )
}

function Shards({ color, speed = 1 }: { color: string; speed?: number }) {
  const shards = useMemo(
    () =>
      Array.from({ length: 7 }, (_, index) => ({
        position: [
          Math.cos((index / 7) * Math.PI * 2) * (3.4 + (index % 3) * 1.1),
          Math.sin(index * 1.7) * 2.1,
          -1.5 + Math.sin((index / 7) * Math.PI * 2) * 3.6,
        ] as [number, number, number],
        scale: 0.1 + (index % 4) * 0.05,
        speed: (1.1 + (index % 3) * 0.4) * speed,
      })),
    [speed],
  )

  return (
    <>
      {shards.map((shard, index) => (
        <Float key={index} speed={shard.speed} rotationIntensity={1.4} floatIntensity={1.6}>
          <mesh position={shard.position} scale={shard.scale}>
            <octahedronGeometry args={[1, 0]} />
            <meshBasicMaterial color={color} wireframe transparent opacity={0.65} />
          </mesh>
        </Float>
      ))}
    </>
  )
}

/** Soft drifting blobs — the "bokeh" scenery. */
function Bokeh({ colors, speed = 1 }: { colors: string[]; speed?: number }) {
  const group = useRef<THREE.Group>(null)
  const blobs = useMemo(
    () =>
      Array.from({ length: 26 }, (_, index) => ({
        position: [
          (Math.random() - 0.5) * 22,
          (Math.random() - 0.5) * 13,
          -3 - Math.random() * 10,
        ] as [number, number, number],
        radius: 0.22 + Math.random() * 0.85,
        color: colors[index % colors.length],
        drift: 0.4 + Math.random(),
      })),
    [colors],
  )

  useFrame((state) => {
    if (!group.current) return
    const t = state.clock.elapsedTime * speed
    group.current.rotation.y = Math.sin(t * 0.05) * 0.12
    group.current.children.forEach((child, index) => {
      child.position.y = blobs[index].position[1] + Math.sin(t * 0.3 * blobs[index].drift + index) * 0.6
    })
  })

  return (
    <group ref={group}>
      {blobs.map((blob, index) => (
        <mesh key={index} position={blob.position}>
          <sphereGeometry args={[blob.radius, 20, 20]} />
          <meshBasicMaterial color={blob.color} transparent opacity={0.14} blending={THREE.AdditiveBlending} depthWrite={false} />
        </mesh>
      ))}
    </group>
  )
}

/** A big billowing shape — the "nebula" scenery. */
function Nebula({ colors, speed = 1 }: { colors: string[]; speed?: number }) {
  const group = useRef<THREE.Group>(null)
  useFrame((state) => {
    if (!group.current) return
    const t = state.clock.elapsedTime * speed
    group.current.rotation.y = t * 0.04
    group.current.children.forEach((child, index) => {
      child.scale.setScalar(1 + Math.sin(t * 0.25 + index) * 0.06)
    })
  })

  return (
    <group ref={group} position={[0, 0, -6]}>
      {colors.map((color, index) => (
        <mesh key={index} position={[(index - 1) * 4.2, (index % 2) * 1.8 - 0.6, -index * 1.6]}>
          <sphereGeometry args={[3.4 + index * 0.5, 24, 24]} />
          <meshBasicMaterial color={color} transparent opacity={0.1} blending={THREE.AdditiveBlending} depthWrite={false} />
        </mesh>
      ))}
    </group>
  )
}

/** A displaced wireframe plane — the "waves" scenery. */
function Waves({ color, speed = 1 }: { color: string; speed?: number }) {
  const geometry = useMemo(() => new THREE.PlaneGeometry(34, 23, 46, 30), [])

  useFrame((state) => {
    const t = state.clock.elapsedTime * speed
    const position = geometry.attributes.position as THREE.BufferAttribute
    for (let index = 0; index < position.count; index += 1) {
      const x = position.getX(index)
      const y = position.getY(index)
      position.setZ(index, Math.sin(x * 0.32 + t) * Math.cos(y * 0.28 + t * 0.8) * 1.15)
    }
    position.needsUpdate = true
  })

  return (
    <mesh geometry={geometry} rotation={[-Math.PI / 2.5, 0, 0]} position={[0, -3, -4]}>
      <meshBasicMaterial color={color} wireframe transparent opacity={0.3} />
    </mesh>
  )
}

/** Animated background built with React Three Fiber. */
export default function Backdrop({
  accent = '#55e6c1',
  light = false,
  intensity = 1,
  speed = 1,
  style = 'grid',
}: BackdropProps) {
  const secondary = light ? '#3f6fd8' : '#6ea8ff'
  const tertiary = light ? '#9a7ae0' : '#b07cff'

  if (cssOnly.includes(style)) {
    return <div className="backdrop" aria-hidden data-style={style} />
  }
  const plain = <div className="backdrop" aria-hidden data-style="minimal" />
  if (!webglAvailable()) return plain

  return (
    <SceneryBoundary fallback={plain}>
    <div className="backdrop" aria-hidden data-style={style}>
      <Canvas
        dpr={[1, 2]}
        camera={{ position: [0, 0.6, 11], fov: 46 }}
        gl={{ antialias: true, alpha: true, powerPreference: 'high-performance' }}
      >
        <fog attach="fog" args={[light ? '#e6ecfa' : '#05070f', 9, 26]} />
        <ambientLight intensity={0.6} />
        <pointLight position={[4, 5, 4]} intensity={26 * intensity} color={accent} />
        <pointLight position={[-6, -3, 2]} intensity={18 * intensity} color={secondary} />

        {style === 'grid' && (
          <>
            <Core color={secondary} speed={speed} />
            <Halo color={accent} speed={speed} />
            <Shards color={secondary} speed={speed} />
            <Particles color={light ? '#7f93bd' : '#9fc0ff'} speed={speed} />
            <Float speed={0.6 * speed} rotationIntensity={0.2} floatIntensity={0.5}>
              <group position={[0, -2.9, 0]}>
                <Grid
                  args={[40, 40]}
                  cellSize={1}
                  cellThickness={0.6}
                  cellColor={light ? '#b9c6e4' : '#1b2b4a'}
                  sectionSize={5}
                  sectionThickness={1}
                  sectionColor={light ? '#8fa6d4' : '#2f4c7d'}
                  fadeDistance={26}
                  fadeStrength={1.6}
                  infiniteGrid
                />
              </group>
            </Float>
          </>
        )}

        {style === 'stars' && (
          <>
            <Particles count={1500} color={light ? '#93a7cc' : '#dfe9ff'} speed={speed * 0.5} />
            <mesh position={[0, 0, -14]}>
              <sphereGeometry args={[9, 32, 32]} />
              <meshBasicMaterial color={tertiary} transparent opacity={0.05} side={THREE.BackSide} />
            </mesh>
          </>
        )}

        {style === 'nebula' && (
          <>
            <Nebula colors={[accent, secondary, tertiary]} speed={speed} />
            <Particles count={700} color={light ? '#9fb0d0' : '#c9d8ff'} speed={speed * 0.6} />
          </>
        )}

        {style === 'waves' && (
          <>
            <Waves color={accent} speed={speed} />
            <Waves color={secondary} speed={speed * 0.6} />
            <Particles count={420} color={light ? '#8fa3c9' : '#a9c6ff'} speed={speed * 0.5} />
          </>
        )}

        {style === 'bokeh' && (
          <>
            <Bokeh colors={[accent, secondary, tertiary]} speed={speed} />
            <Particles count={320} color={light ? '#93a7cc' : '#e6eeff'} speed={speed * 0.4} />
          </>
        )}
      </Canvas>
    </div>
    </SceneryBoundary>
  )
}
