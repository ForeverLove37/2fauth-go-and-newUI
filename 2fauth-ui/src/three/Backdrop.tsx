import { useMemo, useRef } from 'react'
import { Canvas, useFrame } from '@react-three/fiber'
import { Float, Grid } from '@react-three/drei'
import * as THREE from 'three'

interface BackdropProps {
  accent?: string
  light?: boolean
  intensity?: number
}

/** A slowly drifting particle field. */
function Particles({ count = 520, color }: { count?: number; color: string }) {
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
    points.current.rotation.y = state.clock.elapsedTime * 0.035
    points.current.rotation.x = Math.sin(state.clock.elapsedTime * 0.05) * 0.06
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
function Core({ color }: { color: string }) {
  const mesh = useRef<THREE.Mesh>(null)
  useFrame((state) => {
    if (!mesh.current) return
    const t = state.clock.elapsedTime
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
function Halo({ color }: { color: string }) {
  const group = useRef<THREE.Group>(null)
  useFrame((state) => {
    if (!group.current) return
    group.current.rotation.z = state.clock.elapsedTime * 0.06
    group.current.rotation.x = Math.PI / 2.4 + Math.sin(state.clock.elapsedTime * 0.1) * 0.08
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

function Shards({ color }: { color: string }) {
  const shards = useMemo(
    () =>
      Array.from({ length: 7 }, (_, index) => ({
        position: [
          Math.cos((index / 7) * Math.PI * 2) * (3.4 + (index % 3) * 1.1),
          Math.sin(index * 1.7) * 2.1,
          -1.5 + Math.sin((index / 7) * Math.PI * 2) * 3.6,
        ] as [number, number, number],
        scale: 0.1 + (index % 4) * 0.05,
        speed: 1.1 + (index % 3) * 0.4,
      })),
    [],
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

/** Animated background built with React Three Fiber. */
export default function Backdrop({ accent = '#55e6c1', light = false, intensity = 1 }: BackdropProps) {
  const secondary = light ? '#3f6fd8' : '#6ea8ff'
  return (
    <div className="backdrop" aria-hidden>
      <Canvas
        dpr={[1, 1.75]}
        camera={{ position: [0, 0.6, 11], fov: 46 }}
        gl={{ antialias: true, alpha: true, powerPreference: 'high-performance' }}
      >
        <fog attach="fog" args={[light ? '#e6ecfa' : '#05070f', 9, 26]} />
        <ambientLight intensity={0.6} />
        <pointLight position={[4, 5, 4]} intensity={26 * intensity} color={accent} />
        <pointLight position={[-6, -3, 2]} intensity={18 * intensity} color={secondary} />
        <Core color={secondary} />
        <Halo color={accent} />
        <Shards color={secondary} />
        <Particles color={light ? '#7f93bd' : '#9fc0ff'} />
        <Float speed={0.6} rotationIntensity={0.2} floatIntensity={0.5}>
          <group position={[0, -2.9, 0]} rotation={[0, 0, 0]}>
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
      </Canvas>
    </div>
  )
}
